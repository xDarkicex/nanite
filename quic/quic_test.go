package quic

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Addr == "" {
		t.Fatal("expected default Addr")
	}
	if cfg.HTTP1ReadTimeout == 0 || cfg.HTTP1WriteTimeout == 0 || cfg.HTTP1IdleTimeout == 0 {
		t.Fatal("expected non-zero default timeouts")
	}
}

func TestNewSetsDefaults(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{})
	if s.cfg.Addr == "" {
		t.Fatal("expected default addr to be populated")
	}
	if s.h3 == nil {
		t.Fatal("expected HTTP/3 server to be initialized")
	}
	if s.h1 != nil {
		t.Fatal("expected HTTP/1 fallback to be nil when HTTP1Addr is empty")
	}
}

func TestStartHTTP3RequiresTLSFiles(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{Addr: ":0"})
	if err := s.StartHTTP3(); err == nil {
		t.Fatal("expected TLS file validation error")
	}
}

func TestStartDualRequiresHTTP1Addr(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{Addr: ":0", CertFile: "cert.pem", KeyFile: "key.pem"})
	if err := s.StartDual(); err == nil {
		t.Fatal("expected HTTP1Addr validation error")
	}
}

func TestShutdownWithoutStart(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{Addr: ":0"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("unexpected shutdown error: %v", err)
	}
}
