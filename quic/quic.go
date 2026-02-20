package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/quic-go/quic-go/http3"
)

// Config controls HTTP/3 (QUIC) server behavior.
type Config struct {
	Addr string

	CertFile string
	KeyFile  string
	// Optional prebuilt TLS config. When set, CertFile/KeyFile are ignored.
	TLSConfig *tls.Config

	// Optional HTTP/1.1 and HTTP/2 fallback address for dual-stack mode.
	HTTP1Addr string

	HTTP1ReadTimeout  time.Duration
	HTTP1WriteTimeout time.Duration
	HTTP1IdleTimeout  time.Duration
}

// DefaultConfig returns opinionated transport defaults.
func DefaultConfig() Config {
	return Config{
		Addr:              ":8443",
		HTTP1ReadTimeout:  5 * time.Second,
		HTTP1WriteTimeout: 60 * time.Second,
		HTTP1IdleTimeout:  60 * time.Second,
	}
}

// Server hosts HTTP/3 and optional HTTP/1 fallback using the same handler.
type Server struct {
	handler http.Handler
	cfg     Config

	h3 *http3.Server
	h1 *http.Server
}

// New constructs a QUIC server wrapper for any net/http-compatible handler.
func New(handler http.Handler, cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":8443"
	}
	if cfg.HTTP1ReadTimeout == 0 {
		cfg.HTTP1ReadTimeout = 5 * time.Second
	}
	if cfg.HTTP1WriteTimeout == 0 {
		cfg.HTTP1WriteTimeout = 60 * time.Second
	}
	if cfg.HTTP1IdleTimeout == 0 {
		cfg.HTTP1IdleTimeout = 60 * time.Second
	}

	s := &Server{
		handler: handler,
		cfg:     cfg,
	}

	s.h3 = &http3.Server{
		Addr:      cfg.Addr,
		Handler:   handler,
		TLSConfig: cfg.TLSConfig,
	}

	if cfg.HTTP1Addr != "" {
		s.h1 = &http.Server{
			Addr:         cfg.HTTP1Addr,
			Handler:      handler,
			ReadTimeout:  cfg.HTTP1ReadTimeout,
			WriteTimeout: cfg.HTTP1WriteTimeout,
			IdleTimeout:  cfg.HTTP1IdleTimeout,
		}
	}

	return s
}

// StartHTTP3 starts only the HTTP/3 listener.
func (s *Server) StartHTTP3() error {
	if s.cfg.TLSConfig != nil {
		return s.h3.ListenAndServe()
	}
	if err := ValidateTLSFiles(s.cfg.CertFile, s.cfg.KeyFile); err != nil {
		return err
	}
	return s.h3.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile)
}

// StartDual starts optional HTTP/1 fallback and HTTP/3 on QUIC.
// Deprecated compatibility alias for StartDualAndServe.
func (s *Server) StartDual() error {
	return s.StartDualAndServe()
}

// StartDualAndServe starts HTTP/1 fallback and HTTP/3, returning the first non-shutdown error.
func (s *Server) StartDualAndServe() error {
	if s.h1 == nil {
		return errors.New("quic: HTTP1Addr is required for dual-stack mode")
	}

	errCh := make(chan error, 2)

	go func() {
		err := s.h1.ListenAndServe()
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- fmt.Errorf("quic: http1 server error: %w", err)
	}()

	go func() {
		err := s.StartHTTP3()
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			errCh <- nil
			return
		}
		errCh <- fmt.Errorf("quic: http3 server error: %w", err)
	}()

	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			_ = s.ShutdownGraceful(2 * time.Second)
		}
	}

	return firstErr
}

// Shutdown gracefully stops HTTP/3 and optional HTTP/1 fallback servers.
func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	if s.h3 != nil {
		if err := s.h3.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if s.h1 != nil {
		if err := s.h1.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// ShutdownGraceful stops listeners using a timeout-backed context.
func (s *Server) ShutdownGraceful(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.Shutdown(ctx)
}
