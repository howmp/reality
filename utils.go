package reality

import (
	"bytes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/mattn/go-colorable"
	utls "github.com/refraction-networking/utls"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/hkdf"
)

var (
	ErrVerifyFailed  = errors.New("verify failed")
	ErrDecryptFailed = errors.New("decrypt failed")
	ErrProxyDie      = errors.New("proxy die")
)

var Prefix = []byte("REALITY")

const DefaultExpireSecond = 30

var seqNumerOne = [8]byte{0, 0, 0, 0, 0, 0, 0, 1}

// generateNonce 根据SessionKey和ExpireSecond生成Nonce
func generateNonce(NonceSize int, SessionKey []byte, ExpireSecond uint32) ([]byte, error) {
	info := make([]byte, 8)
	binary.BigEndian.PutUint64(info, uint64(time.Now().Unix()%int64(ExpireSecond)))
	nonce := make([]byte, NonceSize)
	_, err := hkdf.New(sha256.New, SessionKey[:], Prefix, info).Read(nonce[:])
	if err != nil {
		return nil, err
	}
	return nonce, nil
}

var versionTLS12 = uint16(utls.VersionTLS12)

const recordHeaderLen = 5
const (
	recordTypeChangeCipherSpec = 20
	recordTypeAlert            = 21
	recordTypeHandshake        = 22
	recordTypeApplicationData  = 23
)

const (
	typeHelloRequest        uint8 = 0
	typeClientHello         uint8 = 1
	typeServerHello         uint8 = 2
	typeNewSessionTicket    uint8 = 4
	typeEndOfEarlyData      uint8 = 5
	typeEncryptedExtensions uint8 = 8
	typeCertificate         uint8 = 11
	typeServerKeyExchange   uint8 = 12
	typeCertificateRequest  uint8 = 13
	typeServerHelloDone     uint8 = 14
	typeCertificateVerify   uint8 = 15
	typeClientKeyExchange   uint8 = 16
	typeFinished            uint8 = 20
	typeCertificateStatus   uint8 = 22
	typeKeyUpdate           uint8 = 24
)

type tlsRecord struct {
	recordType uint8
	version    uint16
	recordData []byte
}

func newTLSRecord(recordType uint8, version uint16, recordData []byte) *tlsRecord {
	return &tlsRecord{
		recordType: recordType,
		version:    version,
		recordData: recordData,
	}
}

func (r *tlsRecord) marshal() []byte {
	data := make([]byte, recordHeaderLen+len(r.recordData))
	data[0] = r.recordType
	data[1] = byte(r.version >> 8)
	data[2] = byte(r.version)
	data[3] = byte(len(r.recordData) >> 8)
	data[4] = byte(len(r.recordData))
	copy(data[5:], r.recordData)
	return data
}

func (r *tlsRecord) writeTo(w io.Writer) (int, error) {
	n, err := bytes.NewReader(r.marshal()).WriteTo(w)
	return int(n), err
}

func readTlsRecord(reader io.Reader) (*tlsRecord, error) {
	hdr := make([]byte, recordHeaderLen)
	if _, err := io.ReadFull(reader, hdr); err != nil {
		return nil, err
	}
	recordType := hdr[0]
	if recordType < recordTypeChangeCipherSpec || recordType > recordTypeApplicationData {
		return nil, errors.New("tls: unknown record type")
	}
	version := uint16(hdr[1])<<8 | uint16(hdr[2])
	if version < utls.VersionTLS10 || version > utls.VersionTLS13 {
		return nil, errors.New("tls: unknown version")
	}
	recordLen := int(hdr[3])<<8 | int(hdr[4])

	recordData := make([]byte, recordLen)
	if _, err := io.ReadFull(reader, recordData); err != nil {
		return nil, err
	}
	return &tlsRecord{
		recordType: recordType,
		version:    version,
		recordData: recordData,
	}, nil
}

const maxSize = 1400
const minSize = 900

var r = rand.New(rand.NewSource(time.Now().UnixNano()))

// generateRandomData 生成随机900-1400数据
func generateRandomData(prefix []byte) []byte {
	len := r.Intn(maxSize-minSize+1) + minSize
	data := make([]byte, len)
	r.Read(data)
	copy(data, prefix)
	return data
}

type OverlayData interface {
	OverlayData() byte
}

var _ OverlayData = (*warpConn)(nil)

type warpConn struct {
	net.Conn
	aead        cipher.AEAD
	overlayData byte
	seq         []byte
	lockRead    *sync.Mutex
	lockWrite   *sync.Mutex
	rawInput    *bytes.Buffer
	maxPayload  int
}

func newWarpConn(conn net.Conn, aead cipher.AEAD, overlayData byte, seq [8]byte) *warpConn {
	incSeq(seq[:])
	w := &warpConn{
		Conn:        conn,
		lockRead:    &sync.Mutex{},
		lockWrite:   &sync.Mutex{},
		rawInput:    &bytes.Buffer{},
		maxPayload:  0xFFFF - aead.Overhead() - recordHeaderLen,
		aead:        aead,
		overlayData: overlayData,
		seq:         seq[:],
	}
	return w
}

func (w *warpConn) Write(b []byte) (int, error) {
	w.lockWrite.Lock()
	defer w.lockWrite.Unlock()
	wrote := 0
	for len(b) > 0 {
		m := len(b)
		if m > w.maxPayload {
			m = w.maxPayload
		}
		data := w.aead.Seal(nil, w.seq[:], b[:m], nil)
		data = append(w.seq[:], data...)
		record := newTLSRecord(recordTypeApplicationData, versionTLS12, data)
		incSeq(w.seq)
		_, err := record.writeTo(w.Conn)
		if err != nil {
			return 0, err
		}
		wrote += m
		b = b[m:]
	}
	return wrote, nil
}

func (w *warpConn) Read(b []byte) (int, error) {
	w.lockRead.Lock()
	defer w.lockRead.Unlock()
	if w.rawInput.Len() != 0 {
		// 缓存中有数据，从缓存返回
		return w.rawInput.Read(b)
	}

	record, err := readTlsRecord(w.Conn)
	if err != nil {
		return 0, err
	}
	if record.recordType != recordTypeApplicationData {
		return 0, ErrVerifyFailed
	}
	if record.version != versionTLS12 {
		return 0, ErrVerifyFailed
	}
	data := record.recordData
	plaintext, err := w.aead.Open(nil, data[:8], data[8:], nil)
	if err != nil {
		return 0, err
	}
	n := copy(b, plaintext)
	if n < len(plaintext) {
		w.rawInput.Write(plaintext[n:])
	}
	return n, nil
}

func (w *warpConn) OverlayData() byte {
	return w.overlayData
}

func incSeq(seq []byte) {
	for i := 7; i >= 0; i-- {
		seq[i]++
		if seq[i] != 0 {
			return
		}
	}
}

func GetLogger(debug bool) logrus.FieldLogger {
	level := logrus.InfoLevel
	if debug {
		level = logrus.DebugLevel
	}
	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetOutput(colorable.NewColorableStderr())
	logger.Formatter = &logrus.TextFormatter{
		ForceColors:      true,
		DisableTimestamp: true,
	}
	return logger
}
