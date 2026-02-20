package quic

import (
	"crypto/tls"
	"errors"
	"fmt"
	"os"
)

// ValidateTLSFiles validates TLS cert/key inputs and existence.
func ValidateTLSFiles(certFile, keyFile string) error {
	if certFile == "" || keyFile == "" {
		return errors.New("quic: CertFile and KeyFile are required")
	}
	if err := validateReadableFile(certFile, "cert"); err != nil {
		return err
	}
	if err := validateReadableFile(keyFile, "key"); err != nil {
		return err
	}
	return nil
}

// LoadTLSConfig loads a certificate pair and returns a TLS config suitable for HTTP/3.
// If base is non-nil, it is cloned and extended.
func LoadTLSConfig(certFile, keyFile string, base *tls.Config) (*tls.Config, error) {
	if err := ValidateTLSFiles(certFile, keyFile); err != nil {
		return nil, err
	}

	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("quic: invalid TLS certificate/key pair: %w", err)
	}

	var cfg *tls.Config
	if base != nil {
		cfg = base.Clone()
	} else {
		cfg = &tls.Config{}
	}

	cfg.Certificates = []tls.Certificate{pair}
	cfg.MinVersion = tls.VersionTLS13
	if !hasProto(cfg.NextProtos, "h3") {
		cfg.NextProtos = append(cfg.NextProtos, "h3")
	}

	return cfg, nil
}

func validateReadableFile(path, kind string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("quic: %s file validation failed (%s): %w", kind, path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("quic: %s path is a directory (%s)", kind, path)
	}
	return nil
}

func hasProto(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
