package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenerateOrLoadTLS returns a TLS config with a self-signed certificate.
// Certificates are cached in the data directory so they persist across restarts.
func GenerateOrLoadTLS(dataDir string) (*tls.Config, error) {
	certFile := filepath.Join(dataDir, "tls-cert.pem")
	keyFile := filepath.Join(dataDir, "tls-key.pem")

	// Try loading existing cert
	if cert, err := tls.LoadX509KeyPair(certFile, keyFile); err == nil {
		log.Println("TLS: loaded existing certificate")
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	// Generate new self-signed cert
	log.Println("TLS: generating self-signed certificate")

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"Fine Print"},
			CommonName:   "Fine Print Local",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add SANs for common local addresses
	template.DNSNames = []string{"localhost", "fineprint.local", "print.local"}
	template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}

	// Add all local network IPs as SANs
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				template.IPAddresses = append(template.IPAddresses, ipnet.IP)
			}
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("creating certificate: %w", err)
	}

	// Save cert PEM
	certOut, err := os.Create(certFile)
	if err != nil {
		return nil, fmt.Errorf("creating cert file: %w", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	// Save key PEM
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshaling key: %w", err)
	}
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return nil, fmt.Errorf("creating key file: %w", err)
	}
	pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	keyOut.Close()

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("loading new cert: %w", err)
	}

	log.Printf("TLS: self-signed certificate generated (SANs: %v)", template.IPAddresses)
	return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
}
