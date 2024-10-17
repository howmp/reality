package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/howmp/reality"
	"github.com/howmp/reality/cmd"
	"github.com/sirupsen/logrus"
)

type serverSession struct {
	config  *reality.ClientConfig
	session *yamux.Session
	logger  logrus.FieldLogger
}

func newServerSession(config *reality.ClientConfig, logger logrus.FieldLogger) *serverSession {
	return &serverSession{
		config: config,
		logger: logger,
	}
}

func (s *serverSession) connectForever() {

	for {
		s.connect()
		s.logger.Infoln("sleep 5s")
		time.Sleep(time.Second * 5)
	}

}
func (s *serverSession) connect() {
	logger := s.logger
	client, err := reality.NewClient(context.Background(), s.config)
	if err != nil {
		logger.Errorf("connect server: %v", err)
		return
	}
	defer client.Close()
	session, err := yamux.Server(client, nil)
	if err != nil {
		logger.Errorf("yamux: %v", err)
		return
	}
	defer session.Close()
	s.session = session
	logger.Infof("session opened %s", client.RemoteAddr())
	<-session.CloseChan()
	logger.Infof("session closed %s", client.RemoteAddr())
}

func (s *serverSession) openSessionStream() (*yamux.Stream, error) {

	if s.session != nil {
		stream, err := s.session.OpenStream()
		if err != nil {
			s.session.Close()
			s.session = nil
			return nil, err
		}
		return stream, nil
	}
	return nil, errors.New("session not open")
}

func main() {
	config, err := reality.UnmarshalClientConfig(cmd.ConfigDataPlaceholder)
	if err != nil {
		println(err.Error())
		return
	}
	logger := reality.GetLogger(config.Debug)
	addr := flag.String("l", "127.0.0.1:61080", "socks5 listen address")
	id := flag.Uint("i", 0, "id")
	flag.Parse()
	logger.Infof("server addr: %s, sni: %s, id: %d", config.ServerAddr, config.SNI, byte(*id))
	config.OverlayData = cmd.NewShortID(false, byte(*id))
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		logger.Panic(err)
	}
	logger.Infof("listen %s", *addr)
	s := newServerSession(config, logger)
	go s.connectForever()
	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Errorf("accept: %v", err)
			continue
		}
		go handleUser(conn, s, logger)
	}
}

func handleUser(conn net.Conn, s *serverSession, logger logrus.FieldLogger) {
	defer conn.Close()

	stream, err := s.openSessionStream()
	if err != nil {
		logger.Errorf("open session stream: %v", err)
		return
	}
	defer stream.Close()

	go io.Copy(stream, conn)
	io.Copy(conn, stream)
}
