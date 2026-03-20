package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"time"
)

func GenerateManagerIdentity() (tls.Certificate, string) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)

	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0), // 10 years
		IsCA:         true,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		log.Fatal(err)
	}

	hash := sha256.Sum256(certDER)
	token := hex.EncodeToString(hash[:])

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, token
}

func GetManagerTlsConfig(tlsCertificate tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{tlsCertificate}}
}

func GetWorkerTlsConfig(tlsToken string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // We manually verify the peer below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			h := sha256.Sum256(rawCerts[0])
			if hex.EncodeToString(h[:]) != tlsToken {
				return fmt.Errorf("SECURITY ALERT: Fingerprint mismatch")
			}
			return nil
		},
	}
}
