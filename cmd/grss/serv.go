package main

import (
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

type sessionManager struct {
	logger       logrus.FieldLogger
	sessions     [128]*yamux.Session
	sessionsLock [128]sync.Mutex
}

func (s *sessionManager) createSession(conn net.Conn, id byte) {
	if s.isSessionOpen(id) {
		s.logger.Errorf("client(id:%d) session already open, close %s", id, conn.RemoteAddr())
		conn.Close()
		return
	}
	s.sessionsLock[id].Lock()
	defer s.sessionsLock[id].Unlock()
	session, err := yamux.Server(conn, nil)
	if err != nil {
		s.logger.Error(err)
		conn.Close()
	}
	go s.checkSession(id, session)
	s.sessions[id] = session
	s.logger.Infof("client(id:%d) session opened %s", id, conn.RemoteAddr())
}

func (s *sessionManager) isSessionOpen(id byte) bool {
	s.sessionsLock[id].Lock()
	defer s.sessionsLock[id].Unlock()
	session := s.sessions[id]
	if session != nil {
		return !session.IsClosed()
	}
	return false
}

func (s *sessionManager) openClientSessionStream(id byte) (*yamux.Stream, error) {
	s.sessionsLock[id].Lock()
	defer s.sessionsLock[id].Unlock()
	session := s.sessions[id]
	if session != nil {
		stream, err := session.OpenStream()
		if err != nil {
			session.Close()
			s.sessions[id] = nil
			return nil, err
		}
		return stream, nil
	}
	return nil, fmt.Errorf("client(id:%d) session not open", id)
}

func (s *sessionManager) checkSession(id byte, session *yamux.Session) {
	<-session.CloseChan()
	s.logger.Infof("client session closed %s", session.RemoteAddr())
	s.sessionsLock[id].Lock()
	defer s.sessionsLock[id].Unlock()
	if s.sessions[id] == session {
		s.sessions[id] = nil
	}
}

// Server 反向socks5代理服务端
type Server struct {
	config *reality.ServerConfig
	logger logrus.FieldLogger
	sm     *sessionManager
}

func NewServer(config *reality.ServerConfig) *Server {
	logger := reality.GetLogger(config.Debug)
	return &Server{
		config: config,
		logger: logger,
		sm: &sessionManager{
			logger: logger,
		},
	}
}

// Serve 监听端口,同时接收Reality客户端和用户连接
func (s *Server) Serve() {
	_, port, err := net.SplitHostPort(s.config.ServerAddr)
	if err != nil {
		s.logger.Fatalf("split ServerAddr %s : %v", s.config.ServerAddr, err)
	}
	bindAddr := fmt.Sprintf(":%s", port)
	l, err := reality.Listen(bindAddr, s.config)
	if err != nil {
		s.logger.Fatalf("reality listen: %v", err)
	}
	s.logger.Infof("reality listen %s", bindAddr)
	for {
		conn, err := l.Accept()
		if err != nil {
			s.logger.Errorf("reality accept: %v", err)
			continue
		}

		if o, ok := conn.(reality.OverlayData); ok {
			isGRSC, id := cmd.ParseShortID(o.OverlayData())
			if isGRSC {
				s.logger.Infof("accept client(id:%d) %s", id, conn.RemoteAddr())

				go s.sm.createSession(conn, id)
				continue
			} else {
				s.logger.Infof("accept user(id:%d) %s", id, conn.RemoteAddr())
				go s.handleUser(conn, id)
				continue
			}
		}
		s.logger.Warnf("accept %s, but no overlay data", conn.RemoteAddr())
		conn.Close()
	}
}

func (s *Server) handleUser(conn net.Conn, id byte) {
	defer conn.Close()

	session, err := yamux.Client(conn, nil)
	if err != nil {
		s.logger.Errorf("user(id:%d) yamux: %v", id, err)
		return
	}
	defer session.Close()
	for {
		stream, err := session.Accept()
		if err != nil {
			s.logger.Errorf("user(id:%d) session accept: %v", id, err)
			return
		}
		s.logger.Infof("user(id:%d) stream accept %s", id, stream.RemoteAddr())
		go s.handleUserStream(stream, id)

	}
}
func (s *Server) handleUserStream(stream net.Conn, id byte) {
	defer stream.Close()
	conn, err := s.sm.openClientSessionStream(id)
	if err != nil {
		s.logger.Errorf("open client(id:%d) session stream: %v", id, err)
		return
	}
	defer conn.Close()
	go io.Copy(conn, stream)
	io.Copy(stream, conn)

}
