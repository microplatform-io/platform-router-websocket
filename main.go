package main

import (
	"encoding/hex"
	"encoding/json"
	"github.com/microplatform-io/platform"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/googollee/go-socket.io"
)

var (
	rabbitUser = os.Getenv("RABBITMQ_USER")
	rabbitPass = os.Getenv("RABBITMQ_PASS")
	rabbitAddr = os.Getenv("RABBITMQ_PORT_5672_TCP_ADDR")
	rabbitPort = os.Getenv("RABBITMQ_PORT_5672_TCP_PORT")
)

type Request struct {
	RequestId string `json:"request_id"`
	Method    int32  `json:"method"`
	Resource  int32  `json:"resource"`
	Protobuf  string `json:"protobuf"`
}

func main() {
	hostname, _ := os.Hostname()

	connMgr := platform.NewAmqpConnectionManager(rabbitUser, rabbitPass, rabbitAddr+":"+rabbitPort, "")

	publisher, err := platform.NewAmqpPublisher(connMgr)
	if err != nil {
		log.Fatalf("> failed to create publisher: %s", err)
	}

	subscriber, err := platform.NewAmqpSubscriber(connMgr, "router_"+hostname)
	if err != nil {
		log.Fatalf("> failed to create subscriber: %s", err)
	}

	standardRouter := platform.NewStandardRouter(publisher, subscriber)

	server, err := socketio.NewServer(nil)
	if err != nil {
		log.Fatal(err)
	}

	server.On("connection", func(so socketio.Socket) {
		log.Println("on connection")

		so.On("request", func(msg string) {
			request := &Request{}
			if err := json.Unmarshal([]byte(msg), request); err != nil {
				log.Println("> failed to decode request:", err)
				return
			}

			protobufBytes, err := hex.DecodeString(request.Protobuf)
			if err != nil {
				log.Println("> failed to decode protobuf hex string:", err)
				return
			}

			routedMessage, err := standardRouter.Route(&platform.RoutedMessage{
				Method:   platform.Int32(request.Method),
				Resource: platform.Int32(request.Resource),
				Body:     protobufBytes,
			}, 5*time.Second)

			// TODO(bmoyles0117):Don't always assume this is a timeout..
			if err != nil {
				errorBytes, _ := platform.Marshal(&platform.Error{
					Message: platform.String("API Request has timed out"),
				})

				routedMessage = &platform.RoutedMessage{
					Method:   platform.Int32(int32(platform.Method_REPLY)),
					Resource: platform.Int32(int32(platform.Resource_ERROR)),
					Body:     errorBytes,
				}
			}

			responseBytes, err := json.Marshal(&Request{
				RequestId: request.RequestId,
				Method:    routedMessage.GetMethod(),
				Resource:  routedMessage.GetResource(),
				Protobuf:  hex.EncodeToString(routedMessage.GetBody()),
			})
			if err != nil {
				log.Println("> failed to encode response:", err)
				return
			}

			so.Emit(request.RequestId, string(responseBytes))
		})

		so.On("disconnection", func() {
			log.Println("on disconnect")
		})
	})

	server.On("error", func(so socketio.Socket, err error) {
		log.Println("error:", err)
	})

	mux := http.NewServeMux()
	mux.Handle("/socket.io/", server)
	mux.Handle("/", http.FileServer(http.Dir("./asset")))

	n := negroni.Classic()
	n.Use(negroni.HandlerFunc(func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Add("Access-Control-Allow-Origin", origin)
		} else {
			w.Header().Add("Access-Control-Allow-Origin", "null")
		}

		w.Header().Add("Access-Control-Allow-Methods", "GET,PUT,POST,DELETE")
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Add("Access-Control-Allow-Credentials", "true")
		w.Header().Add("Connection", "keep-alive")

		next(w, r)
	}))
	n.UseHandler(mux)
	n.Run(":80")
}
