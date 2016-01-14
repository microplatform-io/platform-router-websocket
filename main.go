package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/microplatform-io/platform"
)

const (
	SSL_CERT_FILE = "/tmp/ssl_cert"
	SSL_KEY_FILE  = "/tmp/ssl_key"
)

var (
	rabbitmqEndpoints = strings.Split(os.Getenv("RABBITMQ_ENDPOINTS"), ",")
	routerPort        = platform.Getenv("PORT", "80")
)

type MultiRouter struct {
	routers []platform.Router

	offset int
}

func (r *MultiRouter) Route(request *platform.Request) (chan *platform.Request, chan interface{}) {
	defer r.incrementOffset()

	return r.routers[r.offset].Route(request)
}

func (r *MultiRouter) RouteWithTimeout(request *platform.Request, timeout time.Duration) (chan *platform.Request, chan interface{}) {
	defer r.incrementOffset()

	return r.routers[r.offset].RouteWithTimeout(request, timeout)
}

func (r *MultiRouter) SetHeartbeatTimeout(heartbeatTimeout time.Duration) {
	for i := range r.routers {
		r.routers[i].SetHeartbeatTimeout(heartbeatTimeout)
	}
}

func (r *MultiRouter) incrementOffset() {
	r.offset = (r.offset + 1) % len(r.routers)
}

func NewMultiRouter(routers []platform.Router) *MultiRouter {
	return &MultiRouter{
		routers: routers,
	}
}

type ServerConfig struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	hostname, _ := os.Hostname()

	serverIpAddr, err := platform.GetMyIp()
	if err != nil {
		log.Fatal(err)
	}

	routerUri := "router-" + serverIpAddr + "-" + hostname

	routers := []platform.Router{}

	for i := range rabbitmqEndpoints {
		connectionManager := platform.NewAmqpConnectionManagerWithEndpoint(rabbitmqEndpoints[i])
		publisher := getDefaultPublisher(connectionManager)
		subscriber := getDefaultSubscriber(connectionManager, routerUri)

		routers = append(routers, platform.NewStandardRouterWithTopic(publisher, subscriber, routerUri))
	}

	router := NewMultiRouter(routers)
	router.SetHeartbeatTimeout(7 * time.Second)

	if err := ioutil.WriteFile(SSL_CERT_FILE, []byte(strings.Replace(os.Getenv("SSL_CERT"), "\\n", "\n", -1)), 0755); err != nil {
		log.Fatalf("> failed to write SSL cert file: %s", err)
	}

	if err := ioutil.WriteFile(SSL_KEY_FILE, []byte(strings.Replace(os.Getenv("SSL_KEY"), "\\n", "\n", -1)), 0755); err != nil {
		log.Fatalf("> failed to write SSL cert file: %s", err)
	}

	socketioServer, err := CreateSocketioServer(serverIpAddr, router)
	if err != nil {
		log.Fatalf("> failed to create socketio server: %s", err)
	}

	mux := CreateServeMux(&ServerConfig{
		Protocol: "https",
		Host:     formatHostAddress(serverIpAddr),
		Port:     "443", // we just use this here because this is where it reports it
	})
	mux.Handle("/socket.io/", socketioServer)

	ListenForHttpServer(routerUri, mux)
}

func getDefaultPublisher(connectionManager *platform.AmqpConnectionManager) platform.Publisher {
	publisher, err := platform.NewAmqpPublisher(connectionManager)
	if err != nil {
		log.Fatalf("Could not create publisher. %s", err)
	}

	return publisher
}

func getDefaultSubscriber(connectionManager *platform.AmqpConnectionManager, queue string) platform.Subscriber {
	subscriber, err := platform.NewAmqpSubscriber(connectionManager, queue)
	if err != nil {
		log.Fatalf("Could not create subscriber. %s", err)
	}

	return subscriber
}

func formatHostAddress(ip string) string {
	hostAddress := strings.Replace(ip, ".", "-", -1)

	return fmt.Sprintf("%s.%s", hostAddress, "microplatform.io")
}
