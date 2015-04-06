package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/microplatform-io/platform"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/googollee/go-socket.io"
)

var (
	rabbitUser = os.Getenv("RABBITMQ_USER")
	rabbitPass = os.Getenv("RABBITMQ_PASS")
	rabbitAddr = os.Getenv("RABBITMQ_PORT_5672_TCP_ADDR")
	rabbitPort = os.Getenv("RABBITMQ_PORT_5672_TCP_PORT")

	rabbitRegex            = regexp.MustCompile("RABBITMQ_[0-9]_PORT_5672_TCP_(ADDR|PORT)")
	amqpConnectionManagers []*platform.AmqpConnectionManager
)

type Request struct {
	RequestId string `json:"request_id"`
	Method    int32  `json:"method"`
	Resource  int32  `json:"resource"`
	Protobuf  string `json:"protobuf"`
}

func main() {
	hostname, _ := os.Hostname()

	publisher := getDefaultMultiPublisher()

	subscriber := getDefaultMultiSubscriber("router_" + hostname)

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

func getDefaultMultiPublisher() platform.Publisher {

	publishers := []platform.Publisher{}

	connMgrs := getAmqpConnectionManagers()
	for _, connMgr := range connMgrs {

		publisher, err := platform.NewAmqpPublisher(connMgr)
		if err != nil {
			log.Printf("Could not create publisher. %s", err)
			continue
		}
		publishers = append(publishers, publisher)
	}

	if len(publishers) == 0 {

		log.Fatalf("Failed to create a single publisher: %s\n")
	}

	return platform.NewMultiPublisher(publishers...)

}

func getDefaultMultiSubscriber(queue string) platform.Subscriber {
	subscribers := []platform.Subscriber{}
	connMgrs := getAmqpConnectionManagers()
	for _, connMgr := range connMgrs {

		subscriber, err := platform.NewAmqpSubscriber(connMgr, queue)
		if err != nil {
			log.Printf("Could not create subscriber. %s", err)
			continue
		}
		subscribers = append(subscribers, subscriber)
	}

	if len(subscribers) == 0 {
		log.Fatalf("Failed to create a single subscriber: %s\n")
	}

	return platform.NewMultiSubscriber(subscribers...)
}

func getAmqpConnectionManagers() []*platform.AmqpConnectionManager {
	if amqpConnectionManagers != nil {
		return amqpConnectionManagers
	}

	amqpConnectionManagers := []*platform.AmqpConnectionManager{}

	count := 0
	for _, v := range os.Environ() {
		if rabbitRegex.MatchString(v) {
			count++
		}
	}

	if count == 0 { // No match for multiple rabbitmq servers, try and use single rabbitmq environment variables
		amqpConnectionManagers = append(amqpConnectionManagers, platform.NewAmqpConnectionManager(rabbitUser, rabbitPass, rabbitAddr+":"+rabbitPort, ""))
	} else if count%2 == 0 { // looking for a piar or rabbitmq addr and port
		for i := 0; i < count/2; i++ {
			amqpConnectionManagers = append(amqpConnectionManagers, platform.NewAmqpConnectionManager(rabbitUser, rabbitPass, fmt.Sprintf("%s:%s", os.Getenv(fmt.Sprintf("RABBITMQ_%d_PORT_5672_TCP_ADDR", i+1)), os.Getenv(fmt.Sprint("RABBITMQ_%d_PORT_5672_TCP_PORT", i+1))), ""))
		}
	}

	return amqpConnectionManagers
}
