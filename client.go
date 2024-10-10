package reality

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"

	utls "github.com/refraction-networking/utls"
)

type ClientConfig struct {
	ServerAddr      string `json:"server_addr"`
	SNI             string `json:"sni_name"`
	PublicKeyECDH   string `json:"public_key_ecdh"`
	PublicKeyVerify string `json:"public_key_verify"`
	FingerPrint     string `json:"finger_print,omitempty"`
	ExpireSecond    uint32 `json:"expire_second,omitempty"`
	Debug           bool   `json:"debug,omitempty"`

	fingerPrint     *utls.ClientHelloID // 客户端的TLS指纹
	publicKeyECDH   *ecdh.PublicKey     // 用于密钥协商
	publicKeyVerify ed25519.PublicKey   // 用于验证服务器身份
}

var Fingerprints = map[string]*utls.ClientHelloID{
	"chrome":  &utls.HelloChrome_Auto,
	"firefox": &utls.HelloFirefox_Auto,
	"safari":  &utls.HelloSafari_Auto,
	"ios":     &utls.HelloIOS_Auto,
	"android": &utls.HelloAndroid_11_OkHttp,
	"edge":    &utls.HelloEdge_Auto,
	"360":     &utls.Hello360_Auto,
	"qq":      &utls.HelloQQ_Auto,
}

func (config *ClientConfig) Validate() error {
	if config.ServerAddr == "" {
		return errors.New("server ip is empty")
	}
	if config.SNI == "" {
		return errors.New("server name is empty")
	}
	if config.PublicKeyECDH == "" {
		return errors.New("public key ecdh is empty")
	}
	data, err := base64.StdEncoding.DecodeString(config.PublicKeyECDH)
	if err != nil {
		return err
	}
	config.publicKeyECDH, err = ecdh.X25519().NewPublicKey(data)
	if err != nil {
		return err
	}
	data, err = base64.StdEncoding.DecodeString(config.PublicKeyVerify)
	if err != nil {
		return err
	}
	config.publicKeyVerify = ed25519.PublicKey(data)

	if len(data) != ed25519.PublicKeySize {
		return errors.New("public key verify length error")
	}
	if f, ok := Fingerprints[config.FingerPrint]; ok {
		config.fingerPrint = f
	} else {
		config.fingerPrint = &utls.HelloChrome_Auto
	}
	if config.ExpireSecond == 0 {
		config.ExpireSecond = DefaultExpireSecond
	}
	return nil
}

func (config *ClientConfig) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zipWrite := zlib.NewWriter(&buf)
	if _, err = zipWrite.Write(data); err != nil {
		return nil, err
	}
	if err = zipWrite.Close(); err != nil {
		return nil, err
	}
	zipData := buf.Bytes()
	if len(zipData) > 1022 {
		return nil, errors.New("config data too large")
	}
	zipDataLen := uint16(len(zipData))
	configData := make([]byte, 1024)
	configData[0] = byte(zipDataLen >> 8)
	configData[1] = byte(zipDataLen & 0xff)
	copy(configData[2:], zipData)

	return configData, nil
}

func UnmarshalClientConfig(configData []byte) (*ClientConfig, error) {
	zipDataLen := uint16(configData[0])<<8 | uint16(configData[1])
	if zipDataLen == 0 || zipDataLen > 1022 {
		return nil, errors.New("invalid config length")
	}
	zipData := configData[2 : zipDataLen+2]
	zipReader, err := zlib.NewReader(bytes.NewReader(zipData))
	if err != nil {
		return nil, err
	}

	zipData, err = io.ReadAll(zipReader)
	if err != nil {
		return nil, err
	}
	var config ClientConfig
	err = json.Unmarshal(zipData, &config)
	if err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &config, nil
}

func NewClient(ctx context.Context, config *ClientConfig, overlayData byte) (net.Conn, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 生成临时私钥
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// 与预共享公钥进行密钥协商，计算会话密钥
	sessionKey, err := priv.ECDH(config.publicKeyECDH)
	if err != nil {
		return nil, err
	}

	// 根据会话密钥生成AEAD
	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCMWithNonceSize(block, 8)
	if err != nil {
		return nil, err
	}
	nonce, err := generateNonce(aead.NonceSize(), sessionKey, config.ExpireSecond)
	if err != nil {
		return nil, err
	}
	// 加密数据
	plaintext := make([]byte, 16)
	if _, err := rand.Read(plaintext); err != nil {
		return nil, err
	}
	// 明文为16字节: REALITY + 随机数据
	copy(plaintext, Prefix)
	// 密文为32字节
	ciphertext := aead.Seal(nil, nonce, plaintext, nil)
	var dial net.Dialer
	conn, err := dial.DialContext(ctx, "tcp", config.ServerAddr)
	if err != nil {
		return nil, err
	}
	logger := GetLogger(config.Debug)
	uconn := utls.UClient(
		conn,
		&utls.Config{
			ServerName:             config.SNI,
			SessionTicketsDisabled: true,
			MaxVersion:             utls.VersionTLS12,
		},
		*config.fingerPrint,
	)
	// 构造Client Hello
	if err := uconn.BuildHandshakeState(); err != nil {
		conn.Close()
		return nil, err
	}
	// 将临时公钥和加密数据发送给服务器，分别占用的Random和SessionId
	hello := uconn.HandshakeState.Hello
	hello.Random = priv.PublicKey().Bytes()
	hello.SessionId = ciphertext

	// 已经做好私有握手准备，此时相关数据如下
	logger.Debugf("random(public for ecdh): %x", priv.PublicKey().Bytes())
	logger.Debugf("sessionId(ciphertext): %x", ciphertext)
	logger.Debugf("sessionKey: %x", sessionKey)
	logger.Debugf("nonce: %x", nonce)
	logger.Debugf("plaintext: %x", plaintext)

	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, err
	}
	state := uconn.ConnectionState()
	logger.Debugf("version: %s,cipher: %s", utls.VersionName(state.Version), utls.CipherSuiteName(state.CipherSuite))
	is12 := state.Version == versionTLS12
	if is12 {
		// 进行我们私有握手，客户端发送附加数据，服务端回复64字节签名数据
		logger.Debugf("overlayData: %x", overlayData)
		// record数据前缀模仿seq
		data := generateRandomData(seqNumerOne[:])
		data[len(data)-1] = overlayData
		record := newTLSRecord(recordTypeApplicationData, versionTLS12, data)
		if _, err := record.writeTo(uconn.GetUnderlyingConn()); err != nil {
			uconn.Close()
			return nil, err
		}
		record, err = readTlsRecord(uconn.GetUnderlyingConn())
		if err != nil {
			return nil, err
		}
		if record.recordType != recordTypeApplicationData {
			uconn.Close()
			return nil, ErrVerifyFailed
		}
		if record.version != versionTLS12 {
			uconn.Close()
			return nil, ErrVerifyFailed
		}
		if len(record.recordData) < (64 + 8) {
			uconn.Close()
			return nil, ErrVerifyFailed
		}
		// 服务端回复64字节签名数据
		signature := record.recordData[8:(64 + 8)]
		logger.Debugf("sign: %x", signature)
		if !ed25519.Verify((ed25519.PublicKey)(config.publicKeyVerify), plaintext, signature) {
			uconn.Close()
			return nil, ErrVerifyFailed
		}
		// 服务端回复验证通过
		logger.Debugln("verify ok")
		return newWarpConn(uconn.GetUnderlyingConn(), aead, overlayData, seqNumerOne), nil
	}
	uconn.Close()
	return nil, ErrVerifyFailed

}
