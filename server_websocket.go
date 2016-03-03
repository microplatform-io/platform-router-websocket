package main

import (
	"encoding/hex"
	"log"
	"strings"
	"time"

	"github.com/microplatform-io/platform"
	"github.com/teltechsystems/go-socket.io"
)

func CreateSocketioServer(serverIpAddr string, router platform.Router) (*socketio.Server, error) {
	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	server.On("connection", func(so socketio.Socket) {
		socketId := so.Id()

		clientIpAddr := so.Request().RemoteAddr

		log.Printf("{socket_id:'%s'} - connected", socketId)
		log.Printf("{socket_id:'%s'} - ip addr of client is : %s", socketId, clientIpAddr)

		so.On("request", func(msg string) {
			log.Printf("{socket_id:'%s'} - got request: %s", socketId, msg)

			platformRequestBytes, err := hex.DecodeString(msg)
			if err != nil {
				log.Printf("{socket_id:'%s'} - failed to decode platform request bytes from hex string: %s", socketId, err)
				return
			}

			platformRequest := &platform.Request{}
			if err := platform.Unmarshal(platformRequestBytes, platformRequest); err != nil {
				log.Printf("{socket_id:'%s'} - failed to unmarshal platform request: %s", socketId, err)
				return
			}

			if platformRequest.Routing == nil {
				platformRequest.Routing = &platform.Routing{}
			}

			if !platform.RouteToSchemeMatches(platformRequest, "microservice") {
				log.Printf("{socket_id:'%s'} - unsupported scheme provided: %s", socketId, platformRequest.Routing.RouteTo)
				return
			}

			platformRequest.Routing.RouteFrom = []*platform.Route{
				&platform.Route{
					Uri: platform.String("client://" + socketId),
					IpAddress: &platform.IpAddress{
						Address: platform.String(strings.SplitN(clientIpAddr, ":", 1)[0]),
					},
				},
			}

			requestUuidPrefix := socketId + "::"

			platformRequest.Uuid = platform.String(requestUuidPrefix + platformRequest.GetUuid())

			responses, timeout := router.Route(platformRequest)

			routeToUri := ""
			if len(platformRequest.Routing.RouteTo) > 0 {
				routeToUri = platformRequest.Routing.RouteTo[len(platformRequest.Routing.RouteTo)-1].GetUri()
			}

			go func() {
				for {
					select {
					case response := <-responses:
						log.Printf("{socket_id:'%s'} - got a response for request: %s - %s", socketId, routeToUri, platformRequest.GetUuid())

						response.Uuid = platform.String(strings.Replace(response.GetUuid(), requestUuidPrefix, "", 1))

						// Strip off the tail for routing
						response.Routing.RouteTo = response.Routing.RouteTo[:len(response.Routing.RouteTo)-1]

						responseBytes, err := platform.Marshal(response)
						if err != nil {
							log.Printf("{socket_id:'%s'} - failed to marshal response: %s - %s - %s", socketId, err, routeToUri, platformRequest.GetUuid())
							return
						}

						startTime := time.Now()

						if err := so.Emit("request", hex.EncodeToString(responseBytes)); err != nil {
							log.Printf("{socket_id:'%s'} - failed to emit response: %s - %s - %s", socketId, err, routeToUri, platformRequest.GetUuid())
							return
						}

						log.Printf("{socket_id:'%s'} - time to emit the response: %s - %d nanoseconds", socketId, routeToUri, time.Now().Sub(startTime).Nanoseconds())

						if response.GetCompleted() {
							log.Printf("{socket_id:'%s'} - got the final response for request: %s - %s", socketId, routeToUri, platformRequest.GetUuid())
							return
						}
					case <-timeout:
						log.Printf("{socket_id:'%s'} - got a timeout for request: %s - %s", socketId, routeToUri, platformRequest.GetUuid())
						return
					}
				}
			}()
		})

		so.On("disconnection", func() {
			log.Printf("{socket_id:'%s'} - disconnected", socketId)
		})
	})

	server.On("error", func(so socketio.Socket, err error) {
		log.Printf("{socket_id:'%s'} - error: %s", so.Id(), err)
	})

	return server, nil
}
