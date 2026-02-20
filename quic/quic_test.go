package quic

import (
	"context"
	"net/http"
	"testing"
	"time"

	quiclib "github.com/quic-go/quic-go"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Addr == "" {
		t.Fatal("expected default Addr")
	}
	if cfg.HTTP1ReadTimeout == 0 || cfg.HTTP1WriteTimeout == 0 || cfg.HTTP1IdleTimeout == 0 {
		t.Fatal("expected non-zero default timeouts")
	}
	if cfg.HTTP1ReadHeaderTimeout == 0 {
		t.Fatal("expected non-zero read-header timeout")
	}
	if cfg.HTTP1MaxHeaderBytes == 0 {
		t.Fatal("expected non-zero max header bytes")
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
	if s.h3.QUICConfig != nil {
		t.Fatal("expected nil QUICConfig by default")
	}
}

func TestNewPassesThroughTransportConfig(t *testing.T) {
	qq := &quiclib.Config{MaxIdleTimeout: 3 * time.Second}
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{
		Addr:                   ":0",
		HTTP1Addr:              ":0",
		QUICConfig:             qq,
		HTTP1ReadHeaderTimeout: time.Second,
		HTTP1MaxHeaderBytes:    2048,
	})

	if s.h3.QUICConfig != qq {
		t.Fatal("expected QUIC config passthrough to be preserved")
	}
	if s.h1 == nil {
		t.Fatal("expected HTTP/1 server to be initialized")
	}
	if s.h1.ReadHeaderTimeout != time.Second {
		t.Fatalf("expected read header timeout=1s, got %v", s.h1.ReadHeaderTimeout)
	}
	if s.h1.MaxHeaderBytes != 2048 {
		t.Fatalf("expected max header bytes=2048, got %d", s.h1.MaxHeaderBytes)
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

func TestStartDualAndServeRequiresTLSFiles(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{Addr: ":0", HTTP1Addr: ":0"})
	if err := s.StartDualAndServe(); err == nil {
		t.Fatal("expected TLS file validation error")
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

func TestShutdownGracefulWithoutStart(t *testing.T) {
	s := New(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), Config{Addr: ":0"})
	if err := s.ShutdownGraceful(time.Second); err != nil {
		t.Fatalf("unexpected graceful shutdown error: %v", err)
	}
}
