package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/howmp/reality"
)

func main() {
	logger := reality.GetLogger(false)
	var config reality.ClientConfig
	jsonData, err := os.ReadFile("config.json")
	if err != nil {
		logger.Panic(err)
	}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		logger.Panic(err)
	}

	if err := config.Validate(); err != nil {
		logger.Panic(err)
	}

	client, err := reality.NewClient(context.Background(), &config, 0)
	if err != nil {
		logger.Panic(err)
	}
	defer client.Close()
	reader := bufio.NewReader(client)
	req, err := http.NewRequest("GET", "https://www.qq.com", nil)
	if err != nil {
		logger.Panic(err)
	}
	for i := 0; i < 10; i++ {
		logger.Infoln("req", i)
		req.URL.Path = fmt.Sprintf("/%d", i)
		err = req.Write(client)
		if err != nil {
			logger.Panic(err)
		}
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			logger.Panic(err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			logger.Panic(err)
		}
		logger.Infoln("resp", string(data))

	}

}
