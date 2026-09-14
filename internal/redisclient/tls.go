package redisclient

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSConfig builds a crypto/tls configuration from the connection settings,
// covering ca-cert / client cert+key / skip-verify, mirroring the original
// SSLTransporter behavior (plan §2.1 TLS row).
func TLSConfig(cfg *Config) (*tls.Config, error) {
	tc := &tls.Config{
		ServerName: cfg.Host,
		MinVersion: tls.VersionTLS12,
	}
	if cfg.SSLIgnoreAllErrors {
		tc.InsecureSkipVerify = true
	}
	if cfg.SSLCACertPath != "" {
		pem, err := os.ReadFile(cfg.SSLCACertPath)
		if err != nil {
			return nil, fmt.Errorf("read ca cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("no valid certificates found in %s", cfg.SSLCACertPath)
		}
		tc.RootCAs = pool
	}
	if cfg.SSLLocalCertPath != "" && cfg.SSLPrivateKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.SSLLocalCertPath, cfg.SSLPrivateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	return tc, nil
}
