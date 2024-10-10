package reality

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/cryptobyte"
)

type ServerConfig struct {
	SNIAddr           string `json:"sni_addr"`
	ServerAddr        string `json:"server_addr"`
	PrivateKeyECDH    string `json:"private_key_ecdh"`
	PrivateKeySign    string `json:"private_key_sign"`
	ExpireSecond      uint32 `json:"expire_second"`
	Debug             bool   `json:"debug"`
	ClientFingerPrint string `json:"finger_print,omitempty"`

	privateKeyECDH *ecdh.PrivateKey
	privateKeySign ed25519.PrivateKey
	sniHost        string
	sniPort        string
}

func NewServerConfig(sniAddr string, serverAddr string) (*ServerConfig, error) {
	privateKeyECDH, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	_, privateKeySign, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sniHost, sniPort, err := net.SplitHostPort(sniAddr)
	if err != nil {
		return nil, err
	}
	return &ServerConfig{
		SNIAddr:        sniAddr,
		ServerAddr:     serverAddr,
		PrivateKeyECDH: base64.StdEncoding.EncodeToString(privateKeyECDH.Bytes()),
		PrivateKeySign: base64.StdEncoding.EncodeToString(privateKeySign),
		ExpireSecond:   DefaultExpireSecond,
		privateKeyECDH: privateKeyECDH,
		privateKeySign: privateKeySign,
		sniHost:        sniHost,
		sniPort:        sniPort,
	}, nil

}

func (c *ServerConfig) Validate() error {
	if c.SNIAddr == "" {
		return errors.New("SNI is required")
	}
	var err error
	c.sniHost, c.sniPort, err = net.SplitHostPort(c.SNIAddr)
	if err != nil {
		return err
	}
	if c.ServerAddr == "" {
		return errors.New("server address is required")
	}
	data, err := base64.StdEncoding.DecodeString(c.PrivateKeyECDH)
	if err != nil {
		return err
	}
	c.privateKeyECDH, err = ecdh.X25519().NewPrivateKey(data)
	if err != nil {
		return err
	}
	data, err = base64.StdEncoding.DecodeString(c.PrivateKeySign)
	if err != nil {
		return err
	}
	if len(data) != ed25519.PrivateKeySize {
		return errors.New("private key sign length error")
	}
	c.privateKeySign = ed25519.PrivateKey(data)

	if c.ExpireSecond == 0 {
		c.ExpireSecond = DefaultExpireSecond
	}

	if c.ClientFingerPrint == "" {
		c.ClientFingerPrint = "chrome"
	}
	return nil
}
func (c *ServerConfig) SNIHost() string {
	return c.sniHost
}

func (c *ServerConfig) SNIPort() string {
	return c.sniPort
}
func (s *ServerConfig) ToClientConfig() *ClientConfig {

	return &ClientConfig{
		SNI:             s.sniHost,
		ServerAddr:      s.ServerAddr,
		PublicKeyECDH:   base64.StdEncoding.EncodeToString(s.privateKeyECDH.PublicKey().Bytes()),
		PublicKeyVerify: base64.StdEncoding.EncodeToString(s.privateKeySign.Public().(ed25519.PublicKey)),
		ExpireSecond:    s.ExpireSecond,
		Debug:           s.Debug,
		FingerPrint:     s.ClientFingerPrint,
	}
}

type Listener struct {
	net.Listener
	config   *ServerConfig
	chanConn chan net.Conn
	chanErr  chan error
	logger   logrus.FieldLogger
}

func Listen(laddr string, config *ServerConfig) (net.Listener, error) {
	inner, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	l := &Listener{
		Listener: inner,
		config:   config,
		chanConn: make(chan net.Conn),
		chanErr:  make(chan error),
		logger:   GetLogger(config.Debug),
	}

	go func() {
		for {
			conn, err := l.Listener.Accept()
			if err != nil {
				l.chanErr <- err
				close(l.chanConn)
				return
			}
			go func() {
				c, err := l.handshake(conn)
				if err != nil {
					if l.config.Debug {
						l.logger.Warnln("handshake", conn.RemoteAddr(), err)
					}
				} else {
					l.chanConn <- c
				}
			}()

		}
	}()
	return l, nil
}
func (l *Listener) Accept() (net.Conn, error) {
	if c, ok := <-l.chanConn; ok {
		return c, nil
	}
	return nil, <-l.chanErr
}

// handshake 尝试处理私有握手,失败则进行客户端和代理目标转发，成功返回加密包装后的客户端连接
func (l *Listener) handshake(clientConn net.Conn) (net.Conn, error) {
	logger := l.logger
	targetConn, err := net.Dial("tcp", l.config.SNIAddr)
	if err != nil {
		return nil, errors.Join(ErrProxyDie, err)
	}
	// bufio.Reader是为了在读数据时，不是一个一个record读取，而是模仿一次性读取尽可能多的record
	// io.TeeReader是为了在读数据时，同时互相转发
	clientReader := bufio.NewReader(io.TeeReader(clientConn, targetConn))
	targetReader := bufio.NewReader(io.TeeReader(targetConn, clientConn))
	var aead cipher.AEAD
	var plaintext []byte
	readClientHello := func() error {
		recordClientHello, err := readTlsRecord(clientReader)
		if err != nil {
			return err
		}
		var random, sessionId []byte
		s := cryptobyte.String(recordClientHello.recordData)
		if !s.Skip(6) || // skip type(1) length(3) version(2)
			!s.ReadBytes(&random, 32) ||
			!s.ReadUint8LengthPrefixed((*cryptobyte.String)(&sessionId)) ||
			len(sessionId) != 32 {
			return fmt.Errorf("invalid client hello: %x", hex.EncodeToString(recordClientHello.recordData))
		}
		logger.Debugf("random(public for ecdh): %x", random)
		logger.Debugf("sessionId(ciphertext): %x", sessionId)
		pub, err := ecdh.X25519().NewPublicKey(random)
		if err != nil {
			return err
		}
		sessionKey, err := l.config.privateKeyECDH.ECDH(pub)
		if err != nil {
			return err
		}
		logger.Debugf("sessionKey: %x", sessionKey)

		block, err := aes.NewCipher(sessionKey)
		if err != nil {
			return err
		}
		aead, err = cipher.NewGCMWithNonceSize(block, 8)
		if err != nil {
			return err
		}
		nonce, err := generateNonce(aead.NonceSize(), sessionKey, l.config.ExpireSecond)
		if err != nil {
			return err
		}
		logger.Debugf("nonce: %x", nonce)

		plaintext, err = aead.Open(nil, nonce, sessionId, nil)
		if err != nil {
			return err
		}
		logger.Debugf("plaintext: %x", plaintext)

		if !bytes.HasPrefix(plaintext, Prefix) {
			return fmt.Errorf("invalid prefix: %x", plaintext[:len(Prefix)])
		}
		logger.Debug("handshake ok")
		return nil
	}
	if err = readClientHello(); err != nil {
		go dup(clientConn, targetConn)
		return nil, errors.Join(ErrVerifyFailed, err)
	}

	if _, err = serverOrder1.wait(targetReader, logger); err != nil {
		go dup(clientConn, targetConn)
		return nil, err
	}

	if _, err = clientOrder.wait(clientReader, logger); err != nil {
		go dup(clientConn, targetConn)
		return nil, err
	}
	records, err := serverOrder2.wait(targetReader, logger)
	if err != nil {
		go dup(clientConn, targetConn)
		return nil, err
	}
	// 客户端和代理目标的tls握手已经完成，可以关闭目标的连接
	targetConn.Close()

	// 获取模拟目标的seq，如果有的话
	seq := [8]byte{}
	copy(seq[:], seqNumerOne[:])
	if len(records) > 0 {
		record := records[len(records)-1]
		recordData := record.recordData
		if len(recordData) > len(seq) {
			copy(seq[:], recordData[:len(seq)])
		}
	}
	logger.Debugf("seqNumer: %x", seq)
	incSeq(seq[:])

	// 读取客户端发送的附加内容
	record, err := readTlsRecord(clientConn)
	if err != nil {
		return nil, err
	}
	overlayData := record.recordData[len(record.recordData)-1]
	logger.Debugf("overlayData: %x", overlayData)

	// 发送服务端签名
	sign := ed25519.Sign(ed25519.PrivateKey(l.config.privateKeySign), plaintext)
	logger.Debugf("sign: %x", sign)
	record = newTLSRecord(
		recordTypeApplicationData, versionTLS12,
		generateRandomData(append(seq[:], sign...)), // record数据前缀模仿seq
	)
	if _, err = record.writeTo(clientConn); err != nil {
		clientConn.Close()
		return nil, err
	}
	return newWarpConn(clientConn, aead, overlayData, seq), nil
}

// dup 转发两个连接
func dup(clientConn net.Conn, proxyConn net.Conn) {
	defer clientConn.Close()
	defer proxyConn.Close()
	go io.Copy(proxyConn, clientConn)
	io.Copy(clientConn, proxyConn)
}

type recordOrders []struct {
	recordType    byte
	handshakeType byte
	optional      bool
}

var serverOrder1 = recordOrders{
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeServerHello,
	},
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeCertificate,
	},
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeServerKeyExchange,
	},
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeServerHelloDone,
	},
}

var clientOrder = recordOrders{
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeCertificate,
		optional:      true,
	},
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeClientKeyExchange,
	},
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeCertificateVerify,
		optional:      true,
	},
	{
		recordType: recordTypeChangeCipherSpec,
	},
	{
		recordType: recordTypeHandshake, // Encrypted Handshake Message(Finished)
	},
}

var serverOrder2 = recordOrders{
	{
		recordType:    recordTypeHandshake,
		handshakeType: typeNewSessionTicket,
		optional:      true,
	},
	{
		recordType: recordTypeChangeCipherSpec,
	},
	{

		recordType: recordTypeHandshake, // Encrypted Handshake Message(Finished)
	},
}

func (orders recordOrders) wait(reader io.Reader, logger logrus.FieldLogger) ([]*tlsRecord, error) {
	records := make([]*tlsRecord, 0, len(orders))
	orderPos := 0
	for {
		record, err := readTlsRecord(reader)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
		for pos := orderPos; pos < len(orders); pos++ {
			o := orders[pos]
			if o.handshakeType != 0 {
				// 需要判断握手类型
				if len(record.recordData) != 0 &&
					record.recordData[0] == o.handshakeType {
					orderPos = pos + 1
					break
				}
			} else {
				orderPos = pos + 1
				break
			}

			if o.optional {
				// 如果当前类型是可选的，继续向下查找
				logger.Debugf("try optional record: %+v", record)
				orderPos = pos + 1
				continue
			} else {
				return nil, fmt.Errorf(
					"invalid record, want %+v, got %d %x,",
					o, record.recordType, record.recordData,
				)
			}
		}
		if orderPos == len(orders) {
			return records, nil
		}
	}

}
