package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"github.com/howmp/reality"
	"github.com/howmp/reality/cmd"
	"github.com/sirupsen/logrus"
)

type serv struct {
	ConfigPath string `short:"o" default:"config.json" description:"server config path"`
}

func (s *serv) Execute(args []string) error {
	config, err := loadConfig(s.ConfigPath)
	if err != nil {
		return err
	}
	server := NewServer(config)
	server.Serve()
	return nil
}

// Server 反向socks5代理服务端
type Server struct {
	config         *reality.ServerConfig
	portClientAddr string
	logger         logrus.FieldLogger
	session        *yamux.Session
	sessionLock    *sync.Mutex
}

func NewServer(config *reality.ServerConfig) *Server {
	return &Server{
		config:         config,
		portClientAddr: fmt.Sprintf(":%s", config.SNIPort()),
		logger:         reality.GetLogger(config.Debug),
		sessionLock:    &sync.Mutex{},
	}
}

// Serve 监听端口,同时接收Reality客户端和用户连接
func (s *Server) Serve() {
	l, err := reality.Listen(s.portClientAddr, s.config)
	if err != nil {
		s.logger.Fatalf("reality listen: %v", err)
	}
	s.logger.Infof("reality listen %s", s.portClientAddr)
	for {
		conn, err := l.Accept()
		if err != nil {
			s.logger.Errorf("reality accept: %v", err)
			continue
		}

		if o, ok := conn.(reality.OverlayData); ok {
			overlayData := o.OverlayData()

			if overlayData == cmd.OverlayGRSC {
				s.logger.Infof("accept client %s", conn.RemoteAddr())
				go s.handleClient(conn)
				continue
			} else if overlayData == cmd.OverlayGRSU {
				s.logger.Infof("accept user %s", conn.RemoteAddr())
				go s.handleUser(conn)
				continue
			}
		}
		s.logger.Warnf("accept %s, but overlay wrong", conn.RemoteAddr())
		conn.Close()
	}
}

func (s *Server) handleClient(conn net.Conn) {
	if s.isSessionOpen() {
		s.logger.Errorf("client session already open, close %s", conn.RemoteAddr())
		conn.Close()
		return
	}
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	session, err := yamux.Server(conn, nil)
	if err != nil {
		s.logger.Error(err)
		conn.Close()
	}
	go s.checkSession(session)
	s.session = session
	s.logger.Infof("session opened %s", conn.RemoteAddr())
}

func (s *Server) handleUser(conn net.Conn) {
	defer conn.Close()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		s.logger.Errorf("yamux: %v", err)
		return
	}
	defer session.Close()
	for {
		stream, err := session.Accept()
		if err != nil {
			s.logger.Errorf("user session accept: %v", err)
			return
		}
		s.logger.Infof("user stream accept %s", stream.RemoteAddr())
		go s.handleUserStream(stream)

	}
}
func (s *Server) handleUserStream(stream net.Conn) {
	defer stream.Close()
	conn, err := s.openClientSessionStream()
	if err != nil {
		s.logger.Errorf("open client session stream: %v", err)
		return
	}
	defer conn.Close()
	go io.Copy(conn, stream)
	io.Copy(stream, conn)

}

func (s *Server) isSessionOpen() bool {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	if s.session != nil {
		return !s.session.IsClosed()
	}
	return false
}

func (s *Server) openClientSessionStream() (*yamux.Stream, error) {
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	if s.session != nil {
		stream, err := s.session.OpenStream()
		if err != nil {
			s.session.Close()
			s.session = nil
			return nil, err
		}
		return stream, nil
	}
	return nil, errors.New("client session not open")
}

func (s *Server) checkSession(session *yamux.Session) {
	<-session.CloseChan()
	s.logger.Infof("client session closed %s", session.RemoteAddr())
	s.sessionLock.Lock()
	defer s.sessionLock.Unlock()
	if s.session == session {
		s.session = nil
	}
}
