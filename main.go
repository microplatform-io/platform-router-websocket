package main

import (
	"crypto/tls"
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
	EXTERNAL_IP        = platform.Getenv("EXTERNAL_IP", "") // In kubernetes, we need to return the service's external IP
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

	externalIP := EXTERNAL_IP
	if externalIP == "" {
		logger.Println("An external IP address was not provided, fetching one now")

		discoveredIP, err := platform.GetMyIp()
		if err != nil {
			logger.Fatal(err)
		}

		externalIP = discoveredIP
	}

	logger.Printf("This router's IP will be known as: %s", externalIP)

	routerUri := "router-" + externalIP + "-" + hostname

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

	socketioServer, err := CreateSocketioServer(externalIP, router)
	if err != nil {
		logger.Fatalf("> failed to create socketio server: %s", err)
	}

	go func() {
		mux := CreateServeMux(&ServerConfig{
			Protocol: "http",
			Host:     formatHostAddress(externalIP),
			Port:     PORT_HTTP,
		}, router)
		mux.Handle("/socket.io/", socketioServer)

		wrappedMux := &AccessControlMiddleware{&LoggingMiddleware{mux}}

		logger.Println("Serving HTTP: " + PORT_HTTP)
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
				Host:     formatHostAddress(externalIP),
				Port:     PORT_HTTPS,
			}, router)
			mux.Handle("/socket.io/", socketioServer)

			wrappedMux := &AccessControlMiddleware{&LoggingMiddleware{mux}}

			logger.Println("Serving HTTPS: " + PORT_HTTPS)
			cfg := &tls.Config{
				MinVersion:               tls.VersionTLS12,
				CurvePreferences:         []tls.CurveID{tls.CurveP521, tls.CurveP384, tls.CurveP256},
				PreferServerCipherSuites: true,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				},
			}
			srv := &http.Server{
				Addr:         ":" + PORT_HTTPS,
				Handler:      wrappedMux,
				TLSConfig:    cfg,
				TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler), 0),
			}
			err = srv.ListenAndServeTLS(certFile.Name(), keyFile.Name())
			logger.Fatalf("HTTPS server has died: %s", err)
		}()
	}

	select {}
}

func formatHostAddress(ip string) string {
	hostAddress := strings.Replace(ip, ".", "-", -1)

	return fmt.Sprintf("%s.%s", hostAddress, "microplatform.io")
}
