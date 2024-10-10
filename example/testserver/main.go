package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/howmp/reality"
)

func main() {
	logger := reality.GetLogger(false)

	if len(os.Args) < 2 {
		log.Panic("usage: ./server 127.0.0.1:443")
	}
	addr := os.Args[1]
	config, err := reality.NewServerConfig("www.qq.com:443", addr)
	if err != nil {
		log.Panic(err)
	}
	config.Debug = true
	jsonData, err := json.MarshalIndent(config.ToClientConfig(), "", "  ")
	if err != nil {
		log.Panic(err)
	}
	os.WriteFile("config.json", jsonData, 0644)
	l, err := reality.Listen(addr, config)
	if err != nil {
		log.Panic(err)
	}
	httpServer := http.Server{
		Addr: addr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Infof("req %s", r.RequestURI)
			fmt.Fprintf(w, "hello")
		}),
	}
	logger.Infof("listen %s", addr)
	httpServer.Serve(l)
}
