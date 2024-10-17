package main

import (
	"context"
	"net"
	"time"

	"github.com/armon/go-socks5"
	"github.com/hashicorp/yamux"
	"github.com/howmp/reality"
	"github.com/howmp/reality/cmd"
	"github.com/sirupsen/logrus"
)

func main() {
	config, err := reality.UnmarshalClientConfig(cmd.ConfigDataPlaceholder)
	if err != nil {
		println(err.Error())
		return
	}
	logger := reality.GetLogger(config.Debug)
	logger.Infof("server addr: %s, sni: %s", config.ServerAddr, config.SNI)

	socksServer, err := socks5.New(&socks5.Config{})
	if err != nil {
		logger.Fatalln(err)
	}
	c := client{logger: logger, config: config, socksServer: socksServer}
	for {
		err = c.serve()
		if err != nil {
			logger.Errorf("serve: %v", err)
		}
		logger.Infoln("sleep 5s")
		time.Sleep(time.Second * 5)

	}
}

type client struct {
	logger      logrus.FieldLogger
	config      *reality.ClientConfig
	socksServer *socks5.Server
}

func (c *client) serve() error {
	c.logger.Infoln("try connect to server")
	client, err := reality.NewClient(context.Background(), c.config)
	if err != nil {
		return err
	}
	c.logger.Infoln("server connected")
	defer client.Close()
	session, err := yamux.Client(client, nil)
	if err != nil {
		return err
	}
	defer session.Close()
	for {
		stream, err := session.Accept()
		if err != nil {
			return err
		}
		c.logger.Infof("new client %s", stream.RemoteAddr())
		go c.handleStream(stream)
	}
}

func (c *client) handleStream(conn net.Conn) {
	defer conn.Close()
	c.socksServer.ServeConn(conn)
}
