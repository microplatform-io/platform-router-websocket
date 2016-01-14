FROM golang:1.5.1

ENV RABBITMQ_ENDPOINTS=amqp://admin:password@127.0.0.1:5672//

EXPOSE 80

ADD . /go/src/github.com/microplatform-io/platform-router-websocket
WORKDIR /go/src/github.com/microplatform-io/platform-router-websocket
RUN go get -v ./...

ENTRYPOINT ["platform-router-websocket"]
