package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	utls "github.com/refraction-networking/utls"

	"github.com/howmp/reality"
	"github.com/howmp/reality/cmd"
	"github.com/sirupsen/logrus"
)

type gen struct {
	Debug           bool   `short:"d" description:"debug"`
	FingerPrint     string `short:"f" default:"chrome" description:"client finger print" choice:"chrome" choice:"firefox" choice:"safari" choice:"ios" choice:"android" choice:"edge" choice:"360" choice:"qq"`
	ExpireSecond    uint32 `short:"e" default:"30" description:"expire second"`
	ConfigPath      string `short:"o" default:"config.json" description:"server config output path"`
	ClientCount     byte   `short:"c" default:"3" description:"client count"`
	SkipVerify      bool   `short:"s" description:"skip client cert verify"`
	ClientOutputDir string `long:"dir" default:"." description:"client output directory"`
	Positional      struct {
		SNIAddr    string `description:"tls server address, e.g. example.com:443"`
		ServerAddr string `description:"server address, e.g. 8.8.8.8:443"`
	} `positional-args:"yes"`

	logger logrus.FieldLogger
}

func (c *gen) Execute(args []string) error {
	c.logger = reality.GetLogger(c.Debug)
	var config *reality.ServerConfig
	var err error
	if c.ClientCount > 128 {
		return errors.New("client count must less than 128")
	} else if c.ClientCount == 0 {
		c.ClientCount = 1
	}
	if c.Positional.SNIAddr == "" || c.Positional.ServerAddr == "" {
		c.logger.Infof("try loading config, path %s", c.ConfigPath)
		config, err = loadConfig(c.ConfigPath)
		if err != nil {
			c.logger.Errorf("config load failed: %v", err)
			return err
		}
		c.logger.Infof("config loaded")
		c.Positional.SNIAddr = config.SNIAddr
		c.Positional.ServerAddr = config.ServerAddr

	} else {
		config, err = c.genConfig()
		if err != nil {
			return err
		}
	}
	if err := c.check(); err != nil {
		return err
	}
	return c.genClient(config.ToClientConfig(0))

}

var cipherSuites = map[uint16]bool{
	utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305:    true,
	utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305:  true,
	utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:   true,
	utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: true,
	utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:   true,
	utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: true,
	utls.TLS_RSA_WITH_AES_128_GCM_SHA256:         true,
	utls.TLS_RSA_WITH_AES_256_GCM_SHA384:         true,
}

func (c *gen) check() error {
	logger := c.logger
	logger.Infoln("checking", c.Positional.SNIAddr)
	conn, err := utls.Dial("tcp", c.Positional.SNIAddr, &utls.Config{})
	if err != nil {
		return err
	}
	defer conn.Close()
	logger.Infoln("connected")
	state := conn.ConnectionState()
	logger.Infof("version: %s, ciphersuite: %s", utls.VersionName(state.Version), utls.CipherSuiteName(state.CipherSuite))
	if state.Version != utls.VersionTLS12 {
		return errors.New("server must use tls 1.2")
	}

	useAead := cipherSuites[state.CipherSuite]
	if !useAead {
		logger.Warnln("not use aead cipher suite")
	}
	logger.Infoln("server satisfied")
	return nil
}

func (c *gen) genConfig() (*reality.ServerConfig, error) {

	c.logger.Infof("generating config, path %s", c.ConfigPath)
	config, err := reality.NewServerConfig(c.Positional.SNIAddr, c.Positional.ServerAddr)
	if err != nil {
		return nil, err
	}

	config.Debug = c.Debug
	config.ClientFingerPrint = c.FingerPrint
	config.ExpireSecond = c.ExpireSecond
	config.SkipVerify = c.SkipVerify
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(c.ConfigPath, data, 0644); err != nil {
		return nil, err
	}
	return config, nil
}
func (c *gen) genClient(clientConfig *reality.ClientConfig) error {
	c.logger.Infof("generating client, path %s", c.ClientOutputDir)
	configData, err := clientConfig.Marshal()
	if err != nil {
		return err
	}

	for _, name := range AssetNames() {
		if strings.HasPrefix(name, "grsc") {
			// 根据客户端数量生成多个客户端
			for i := 0; i < int(c.ClientCount); i++ {
				path := filepath.Join(c.ClientOutputDir, fmt.Sprintf("grsc%d%s", i, name[4:]))
				clientConfig.OverlayData = cmd.NewShortID(true, byte(i))
				clientConfigData, err := clientConfig.Marshal()
				if err != nil {
					return err
				}

				ClientBin, err := replaceClientTemplate(MustAsset(name), clientConfigData)
				if err != nil {
					return err
				}

				if err := os.WriteFile(path, ClientBin, 0755); err != nil {
					return err
				}
				c.logger.Infof("generated %s", path)
			}
			continue
		}
		path := filepath.Join(c.ClientOutputDir, name)
		ClientBin, err := replaceClientTemplate(MustAsset(name), configData)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, ClientBin, 0755); err != nil {
			return err
		}
		c.logger.Infof("generated %s", path)
	}
	return nil
}

func loadConfig(path string) (*reality.ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	config := &reality.ServerConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func replaceClientTemplate(template []byte, configData []byte) ([]byte, error) {
	placeholder := make([]byte, len(cmd.ConfigDataPlaceholder))
	copy(placeholder, []byte{0xff, 0xff, 'g', 'r', 's', 'c', 'o', 'n', 'f', 'i', 'g'})
	pos := bytes.Index(template, placeholder)
	if pos == -1 {
		return nil, errors.New("config placeholder not found")
	}
	buf := bytes.NewBuffer(make([]byte, 0, len(template)))
	buf.Write(template[:pos])
	buf.Write(configData)
	buf.Write(template[pos+len(configData):])
	return buf.Bytes(), nil
}
