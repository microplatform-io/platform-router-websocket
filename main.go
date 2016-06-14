package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/microplatform-io/platform"
	"github.com/microplatform-io/platform/amqp"
)

var (
	RABBITMQ_ENDPOINTS = strings.Split(os.Getenv("RABBITMQ_ENDPOINTS"), ",")
	PORT_HTTP          = platform.Getenv("PORT_HTTP", "80")
	PORT_HTTPS         = platform.Getenv("PORT_HTTPS", "443")
	SSL_CERT           = platform.Getenv("SSL_CERT", "")
	SSL_KEY            = platform.Getenv("SSL_KEY", "")

	logger = platform.GetLogger("platform-router-websocket")
)

type ServerConfig struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
}

func main() {
	hostname, _ := os.Hostname()

	serverIpAddr, err := platform.GetMyIp()
	if err != nil {
		logger.Fatal(err)
	}

	routerUri := "router-" + serverIpAddr + "-" + hostname

	amqpDialers := amqp.NewCachingDialers(RABBITMQ_ENDPOINTS)

	dialerInterfaces := []amqp.DialerInterface{}
	for i := range amqpDialers {
		dialerInterfaces = append(dialerInterfaces, amqpDialers[i])
	}

	publisher, err := amqp.NewMultiPublisher(dialerInterfaces)
	if err != nil {
		logger.Fatalf("> failed to create multi publisher: %s", err)
	}

	subscriber, err := amqp.NewMultiSubscriber(dialerInterfaces, routerUri)
	if err != nil {
		logger.Fatalf("> failed to create multi subscriber: %s", err)
	}

	router := platform.NewStandardRouter(publisher, subscriber)
	router.SetHeartbeatTimeout(7 * time.Second)

	socketioServer, err := CreateSocketioServer(serverIpAddr, router)
	if err != nil {
		logger.Fatalf("> failed to create socketio server: %s", err)
	}

	go func() {
		mux := CreateServeMux(&ServerConfig{
			Protocol: "http",
			Host:     formatHostAddress(serverIpAddr),
			Port:     PORT_HTTP,
		}, router)
		mux.Handle("/socket.io/", socketioServer)

		wrappedMux := &AccessControlMiddleware{&LoggingMiddleware{mux}}

		err := http.ListenAndServe(":"+PORT_HTTP, wrappedMux)
		logger.Fatalf("HTTP server has died: %s", err)
	}()

	if SSL_CERT != "" && SSL_KEY != "" {
		go func() {
			certFile, err := ioutil.TempFile("", "cert")
			if err != nil {
				logger.Fatalf("failed to write cert file: %s", err)
			}
			defer certFile.Close()

			keyFile, err := ioutil.TempFile("", "key")
			if err != nil {
				logger.Fatalf("failed to write key file: %s", err)
			}
			defer keyFile.Chdir()

			logger.Println(certFile.Name(), keyFile.Name())

			ioutil.WriteFile(certFile.Name(), []byte(strings.Replace(SSL_CERT, "\\n", "\n", -1)), os.ModeTemporary)
			ioutil.WriteFile(keyFile.Name(), []byte(strings.Replace(SSL_KEY, "\\n", "\n", -1)), os.ModeTemporary)

			mux := CreateServeMux(&ServerConfig{
				Protocol: "https",
				Host:     formatHostAddress(serverIpAddr),
				Port:     PORT_HTTPS,
			}, router)
			mux.Handle("/socket.io/", socketioServer)

			wrappedMux := &AccessControlMiddleware{&LoggingMiddleware{mux}}

			err = http.ListenAndServeTLS(":"+PORT_HTTPS, certFile.Name(), keyFile.Name(), wrappedMux)
			logger.Fatalf("HTTPS server has died: %s", err)
		}()
	}

	select {}
}

func formatHostAddress(ip string) string {
	hostAddress := strings.Replace(ip, ".", "-", -1)

	return fmt.Sprintf("%s.%s", hostAddress, "microplatform.io")
}
