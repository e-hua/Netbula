package configs

import (
	"crypto/ed25519"
	"crypto/tls"
)

type ManagerConfig struct {
	WorkerConnectionPort int
	ServerApiPort        int
	// TlsCertificate tls.Certificate
	TlsCertificateInBytes [][]byte
	TlsPrivateKey         []byte
	// token: SHA-256 hash of certificate
	TlsToken string
}

func NewManagerConfig(workerConnectionPort int, serverApiPort int, tlsCertificate tls.Certificate, tlsToken string) *ManagerConfig {
	return &ManagerConfig{
		WorkerConnectionPort:  workerConnectionPort,
		ServerApiPort:         serverApiPort,
		TlsCertificateInBytes: tlsCertificate.Certificate,
		TlsPrivateKey:         tlsCertificate.PrivateKey.(ed25519.PrivateKey),
		TlsToken:              tlsToken,
	}
}
