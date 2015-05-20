FROM tutum.co/spoofcard/scratch-teltech

ADD dist/platform-router-websocket /platform-router-websocket

ENTRYPOINT ["/platform-router-websocket"]
