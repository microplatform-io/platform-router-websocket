package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/microplatform-io/platform"
)

var (
	HEALTH_CHECK_PAYLOAD_VARIABLE = "payload"
)

func ListenForHttpServer(routerUri string, mux *http.ServeMux) {
	defer func() {
		if r := recover(); r != nil {
			logger.Println("> http server has died: %s", r)
		}
	}()

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

	n.Run(":" + routerPort)
} 

func CreateServeMux(serverConfig *ServerConfig, router platform.Router) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/server", serverHandler(serverConfig))
	mux.HandleFunc("/healthcheck", healthcheckHandler(router))

	return mux
}

func serverHandler(serverConfig *ServerConfig) func(w http.ResponseWriter, req *http.Request) {
	jsonBytes, _ := json.Marshal(serverConfig)

	return func(w http.ResponseWriter, req *http.Request) {
		cb := req.FormValue("callback")

		if cb == "" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(jsonBytes)
			return
		}

		w.Header().Set("Content-Type", "application/javascript")
		fmt.Fprintf(w, fmt.Sprintf("%s(%s)", cb, jsonBytes))
	}
}

func healthcheckHandler(router platform.Router) func(w http.ResponseWriter, req *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		requestUrlQuery := req.URL.Query()
		if len(requestUrlQuery) == 0 {
			w.WriteHeader(http.StatusOK)
			return
		}

		payload := requestUrlQuery.Get(HEALTH_CHECK_PAYLOAD_VARIABLE)

		payloadBytes, err := hex.DecodeString(payload)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		platformRequest := &platform.Request{}
		err = platform.Unmarshal(payloadBytes, platformRequest)
		if err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		platformRequest.Uuid = platform.String(platform.CreateUUID())

		responses, timeout := router.Route(platformRequest)

		for {
			select {
			case response := <-responses:
				if response.GetCompleted() {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Ok"))
					return
				}

			case <-time.After(1 * time.Second):
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				return

			case <-timeout:
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				return
			}
		}
	}
}
