package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
)

func handshake() {
	conn, err := tls.Dial("tcp", "www.qq.com:443", &tls.Config{})
	if err != nil {
		panic(err)
	}
	err = conn.Handshake()
	if err != nil {
		panic(err)
	}
	// conn.Close()
}

func main() {
	signTest()
	signTest2()
	authkeyTest()
	plaintext := make([]byte, 32)
	if _, err := rand.Read(plaintext); err != nil {
		panic(err)
	}
	copy(plaintext, []byte("REALITY"))
	fmt.Println("plaintext", len(plaintext), plaintext)
}

func signTest() {
	fmt.Println("signTest")
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	data, err := x509.MarshalECPrivateKey(privateKey)
	fmt.Println("priv", base64.RawURLEncoding.EncodeToString(data))
	if err != nil {
		panic(err)
	}
	data, err = x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		panic(err)
	}
	fmt.Println("publ", base64.RawURLEncoding.EncodeToString(data))
	hash := []byte("hello")
	sign, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		panic(err)
	}
	fmt.Println("sign", len(sign), base64.RawURLEncoding.EncodeToString(sign))
	ok := ecdsa.VerifyASN1(&privateKey.PublicKey, hash[:], sign)
	fmt.Println("verify", ok)

}
func signTest2() {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}
	fmt.Println("priv", base64.RawURLEncoding.EncodeToString(priv))
	fmt.Println("publ", base64.RawURLEncoding.EncodeToString(pub))

	msg := []byte("1")

	sign := ed25519.Sign(priv, msg)
	fmt.Println("sign", len(sign), base64.RawURLEncoding.EncodeToString(sign))
	ok := ed25519.Verify(pub, msg, sign)
	fmt.Println("verify", ok)

}

func authkeyTest() {
	fmt.Println("authkeyTest")
	// 服务端持有私钥，客户端持有公钥
	privateKeyServer, publicKeyClient := genEcdhX25519()
	fmt.Println("priv", len(privateKeyServer), base64.RawURLEncoding.EncodeToString(privateKeyServer))
	fmt.Println("publ", len(publicKeyClient), base64.RawURLEncoding.EncodeToString(publicKeyClient))
	// tmp是每次Client Hello生成,其中公钥放在SessionID，私钥在客户端内存中
	privateKeyTmp, publicKeyTmp := genEcdhX25519()

	// 服务端进行密钥协商，得到authkey
	authkey1, err := ecdhAuthKey(privateKeyServer, publicKeyTmp)
	if err != nil {
		panic(err)
	}

	// 客户端进行密钥协商，得到authkey
	authkey2, err := ecdhAuthKey(privateKeyTmp, publicKeyClient)
	if err != nil {
		panic(err)
	}
	fmt.Println(base64.RawURLEncoding.EncodeToString(authkey1))
	fmt.Println(base64.RawURLEncoding.EncodeToString(authkey2))

	block, err := aes.NewCipher(authkey2)
	if err != nil {
		panic(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	fmt.Println("overhead", aead.Overhead(), "noncesize", aead.NonceSize())
	ciphertext := aead.Seal(nil, make([]byte, aead.NonceSize()), []byte("1234567890123456"), nil)
	fmt.Println(len(ciphertext), base64.RawURLEncoding.EncodeToString(ciphertext))

}

func ecdhAuthKey(privateKey []byte, publicKey []byte) ([]byte, error) {

	priv, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	pub, err := ecdh.X25519().NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	return priv.ECDH(pub)
}

func genEcdhX25519() ([]byte, []byte) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return priv.Bytes(), priv.PublicKey().Bytes()

}
