package network

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"log"
	"math/big"
	"net"
	"time"
)

// TLSDial 建立到代理端的 TLS 连接。
// enable 为 false 时退化为普通 TCP 连接。
// 客户端默认不校验服务端证书（InsecureSkipVerify），与 frp 默认行为一致，
// 以便在使用自签证书时开箱即用。
func TLSDial(address string, enable bool) (net.Conn, error) {
	if !enable {
		return net.DialTimeout("tcp", address, time.Second*90)
	}
	return tls.Dial("tcp", address, &tls.Config{
		InsecureSkipVerify: true,
	})
}

// TLSListen 在 address 上建立监听。
// enable 为 false 时退化为普通 TCP 监听。
// certFile/keyFile 为空时自动生成内存自签证书，保证零配置可用。
func TLSListen(address, certFile, keyFile string, enable bool) (net.Listener, error) {
	if !enable {
		return net.Listen("tcp", address)
	}

	var cert tls.Certificate
	var err error
	if certFile != "" && keyFile != "" {
		cert, err = tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		log.Printf("TLS 已启用，使用证书 %s", certFile)
	} else {
		cert, err = generateSelfSignedCert()
		if err != nil {
			return nil, err
		}
		log.Println("TLS 已启用，使用自动生成的自签证书")
	}

	return tls.Listen("tcp", address, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
}

// generateSelfSignedCert 在内存中生成一张自签证书（ECDSA P-256），不落盘。
func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject:      pkix.Name{Organization: []string{"ruijia-tunlix"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
	}, nil
}