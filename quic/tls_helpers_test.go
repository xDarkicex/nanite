package quic

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateTLSFiles(t *testing.T) {
	if err := ValidateTLSFiles("", ""); err == nil {
		t.Fatal("expected empty-path validation error")
	}
	if err := ValidateTLSFiles("/does/not/exist.crt", "/does/not/exist.key"); err == nil {
		t.Fatal("expected missing file validation error")
	}
}

func TestLoadTLSConfig(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")

	if err := writeSelfSignedPair(certFile, keyFile); err != nil {
		t.Fatalf("failed to generate test certs: %v", err)
	}

	cfg, err := LoadTLSConfig(certFile, keyFile, nil)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS min version to be set to TLS1.3, got %d", cfg.MinVersion)
	}
	found := false
	for _, proto := range cfg.NextProtos {
		if proto == "h3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected h3 in NextProtos")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected one certificate, got %d", len(cfg.Certificates))
	}
}

func writeSelfSignedPair(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}

	keyOut, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return err
	}

	return nil
}
