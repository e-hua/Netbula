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

// Getting a random 32-characters-long string in hexadecimal encoding
func GenerateRandomToken() string {
	bytes := make([]byte, 32)

	// Never returns an error
	rand.Read(bytes)

	hexEncodedString := hex.EncodeToString(bytes)

	return hexEncodedString
}

func GenerateManagerIdentity() (tls.Certificate, string, string) {
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

	certHash := sha256.Sum256(certDER)
	certEncodedHash := hex.EncodeToString(certHash[:])

	token := GenerateRandomToken()

	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}, token, certEncodedHash
}

func GetManagerTlsConfig(tlsCertificate tls.Certificate) *tls.Config {
	return &tls.Config{Certificates: []tls.Certificate{tlsCertificate}}
}

// Creates TLS config for a specific hash of server certificate
// Reject other certificates with different hash
func GenerateTlsConfig(certHash string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true, // We manually verify the peer below
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			h := sha256.Sum256(rawCerts[0])
			if hex.EncodeToString(h[:]) != certHash {
				return fmt.Errorf("SECURITY ALERT: Fingerprint mismatch, the TLS certificate sent by the server has the wrong hash")
			}
			return nil
		},
	}
}
