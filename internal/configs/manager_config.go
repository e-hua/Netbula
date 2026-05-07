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
	// token: random-generated hex-encoded string
	TlsToken string
	// fingerprint: SHA-256 hash of certificate,
	// Used by worker and control program to validate the certificate of TLS connection
	CertFingerprint string
}

func NewManagerConfig(workerConnectionPort int, serverApiPort int, tlsCertificate tls.Certificate, tlsToken string, certHash string) *ManagerConfig {
	return &ManagerConfig{
		WorkerConnectionPort:  workerConnectionPort,
		ServerApiPort:         serverApiPort,
		TlsCertificateInBytes: tlsCertificate.Certificate,
		TlsPrivateKey:         tlsCertificate.PrivateKey.(ed25519.PrivateKey),
		TlsToken:              tlsToken,
		CertFingerprint:       certHash,
	}
}
