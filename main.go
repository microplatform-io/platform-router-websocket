package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/microplatform-io/platform"
)

const (
	SSL_CERT_FILE = "/tmp/ssl_cert"
	SSL_KEY_FILE  = "/tmp/ssl_key"
)

var (
	rabbitUser = os.Getenv("RABBITMQ_USER")
	rabbitPass = os.Getenv("RABBITMQ_PASS")
	rabbitAddr = os.Getenv("RABBITMQ_PORT_5672_TCP_ADDR")
	rabbitPort = os.Getenv("RABBITMQ_PORT_5672_TCP_PORT")

	routerPort = platform.Getenv("PORT", "443")
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

	connectionManager := platform.NewAmqpConnectionManager(rabbitUser, rabbitPass, rabbitAddr+":"+rabbitPort, "")

	router := platform.NewStandardRouter(getDefaultPublisher(connectionManager), getDefaultSubscriber(connectionManager, routerUri))
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

	manageRouterState(&platform.RouterConfigList{
		RouterConfigs: []*platform.RouterConfig{
			&platform.RouterConfig{
				RouterType:   platform.RouterConfig_ROUTER_TYPE_WEBSOCKET.Enum(),
				ProtocolType: platform.RouterConfig_PROTOCOL_TYPE_HTTPS.Enum(),
				Host:         platform.String(serverIpAddr),
				Port:         platform.String(routerPort),
			},
		},
	})

	mux := CreateServeMux(&ServerConfig{
		Protocol: "http",
		Host:     formatHostAddress(serverIpAddr),
		Port:     routerPort, // we just use this here because this is where it reports it
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

func manageRouterState(routerConfigList *platform.RouterConfigList) {
	log.Printf("%+v", routerConfigList)

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Emit a router offline signal if we catch an interrupt
	go func() {
		select {
		case <-sigc:
			// TODO: UPDATE ETCD WITH OFFLINE

			os.Exit(0)
		}
	}()

	// Wait for the servers to come online, and then repeat the router.online every 30 seconds
	time.AfterFunc(10*time.Second, func() {
		// TODO: UPDATE ETCD WITH ONLINE

		for {
			time.Sleep(30 * time.Second)
			// TODO: UPDATE ETCD WITH ONLINE
		}
	})
}
