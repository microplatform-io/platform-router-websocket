package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
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

type MultiPublisher struct {
	publishers []platform.Publisher

	offset int
}

func (p *MultiPublisher) Publish(topic string, body []byte) error {
	defer p.incrementOffset()

	return p.publishers[p.offset].Publish(topic, body)
}

func (p *MultiPublisher) incrementOffset() {
	p.offset = (p.offset + 1) % len(p.publishers)
}

func NewMultiPublisher(publishers []platform.Publisher) *MultiPublisher {
	return &MultiPublisher{
		publishers: publishers,
	}
}

type MultiSubscriber struct {
	subscribers []platform.Subscriber
}

func (s *MultiSubscriber) Run() error {
	wg := &sync.WaitGroup{}
	wg.Add(2)

	for i := range s.subscribers {
		go func(i int) {
			s.subscribers[i].Run()

			wg.Done()
		}(i)
	}

	wg.Wait()

	return nil
}

func (s *MultiSubscriber) Subscribe(topic string, handler platform.ConsumerHandler) {
	for i := range s.subscribers {
		s.subscribers[i].Subscribe(topic, handler)
	}
}

func NewMultiSubscriber(subscribers []platform.Subscriber) *MultiSubscriber {
	return &MultiSubscriber{
		subscribers: subscribers,
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

	publishers := []platform.Publisher{}
	subscribers := []platform.Subscriber{}

	for i := range rabbitmqEndpoints {
		connectionManager := platform.NewAmqpConnectionManagerWithEndpoint(rabbitmqEndpoints[i])

		publishers = append(publishers, getDefaultPublisher(connectionManager))
		subscribers = append(subscribers, getDefaultSubscriber(connectionManager, routerUri))
	}

	router := platform.NewStandardRouter(NewMultiPublisher(publishers), NewMultiSubscriber(subscribers))
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
	}, router)
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
