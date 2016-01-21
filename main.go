package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/microplatform-io/platform"
)

var (
	rabbitmqEndpoints = strings.Split(os.Getenv("RABBITMQ_ENDPOINTS"), ",")
	routerPort        = platform.Getenv("PORT", "80")
)

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

	connectionManagers := platform.NewAmqpConnectionManagersWithEndpoints(rabbitmqEndpoints)

	publisher, err := platform.NewAmqpMultiPublisher(connectionManagers)
	if err != nil {
		log.Fatalf("> failed to create multi publisher: %s", err)
	}

	subscriber, err := platform.NewAmqpMultiSubscriber(connectionManagers, routerUri)
	if err != nil {
		log.Fatalf("> failed to create multi subscriber: %s", err)
	}

	router := platform.NewStandardRouter(publisher, subscriber)
	router.SetHeartbeatTimeout(7 * time.Second)

	socketioServer, err := CreateSocketioServer(serverIpAddr, router)
	if err != nil {
		log.Fatalf("> failed to create socketio server: %s", err)
	}

	mux := CreateServeMux(&ServerConfig{
		Protocol: "https",
		Host:     formatHostAddress(serverIpAddr),
		Port:     "443", // we just use this here because this is where it reports it
	}, router)
	mux.Handle("/socket.io/", socketioServer)
	ListenForHttpServer(routerUri, mux)
}

func formatHostAddress(ip string) string {
	hostAddress := strings.Replace(ip, ".", "-", -1)

	return fmt.Sprintf("%s.%s", hostAddress, "microplatform.io")
}
