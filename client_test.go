package reality_test

import (
	"context"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/howmp/reality"
)

func TestClient(t *testing.T) {

	privEcdh, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubVerify, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	config := &reality.ClientConfig{
		SNI:             "www.qq.com",
		ServerAddr:      "www.qq.com:443",
		PublicKeyECDH:   base64.StdEncoding.EncodeToString(privEcdh.Bytes()),
		PublicKeyVerify: base64.StdEncoding.EncodeToString(pubVerify),
		Debug:           true,
	}
	d, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(d))

	_, err = reality.NewClient(context.Background(), config, 0)
	if err == nil {
		t.Fatal("should error")
	}

}

func TestClientConfig(t *testing.T) {
	configServer, err := reality.NewServerConfig("example.com:443", "127.0.0.1:443")
	if err != nil {
		t.Fatal(err)
	}
	config := configServer.ToClientConfig()
	configData, err := config.Marshal()
	if err != nil {
		t.Fatal(err)
	}

	newConfig, err := reality.UnmarshalClientConfig(configData)
	if err != nil {
		t.Fatal(err)
	}

	if err := newConfig.Validate(); err != nil {
		t.Fatal(err)
	}

}
