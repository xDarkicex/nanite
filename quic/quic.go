package quic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"time"

	quiclib "github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// Config controls HTTP/3 (QUIC) server behavior.
type Config struct {
	Addr string

	CertFile string
	KeyFile  string
	// Optional prebuilt TLS config. When set, CertFile/KeyFile are ignored.
	TLSConfig *tls.Config
	// Optional passthrough for QUIC transport tuning.
	QUICConfig *quiclib.Config

	// Optional HTTP/1.1 and HTTP/2 fallback address for dual-stack mode.
	HTTP1Addr string

	HTTP1ReadTimeout       time.Duration
	HTTP1ReadHeaderTimeout time.Duration
	HTTP1WriteTimeout      time.Duration
	HTTP1IdleTimeout       time.Duration
	HTTP1MaxHeaderBytes    int
	HTTP1DisableKeepAlives bool

	// Optional structured lifecycle callback.
	Logger LoggerFunc
}

// DefaultConfig returns opinionated transport defaults.
func DefaultConfig() Config {
	return Config{
		Addr:                   ":8443",
		HTTP1ReadTimeout:       5 * time.Second,
		HTTP1ReadHeaderTimeout: 5 * time.Second,
		HTTP1WriteTimeout:      60 * time.Second,
		HTTP1IdleTimeout:       60 * time.Second,
		HTTP1MaxHeaderBytes:    1 << 20,
	}
}

// Server hosts HTTP/3 and optional HTTP/1 fallback using the same handler.
type Server struct {
	handler http.Handler
	cfg     Config

	h3 *http3.Server
	h1 *http.Server
}

// Event carries structured lifecycle signals for observability hooks.
type Event struct {
	Time      time.Time
	Component string // "h1", "h3", or "server"
	Status    string // "started", "stopped", or "error"
	Addr      string
	Err       error
}

// LoggerFunc receives structured lifecycle events.
type LoggerFunc func(Event)

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
	if cfg.HTTP1ReadHeaderTimeout == 0 {
		cfg.HTTP1ReadHeaderTimeout = 5 * time.Second
	}
	if cfg.HTTP1MaxHeaderBytes == 0 {
		cfg.HTTP1MaxHeaderBytes = 1 << 20
	}

	s := &Server{
		handler: handler,
		cfg:     cfg,
	}

	s.h3 = &http3.Server{
		Addr:       cfg.Addr,
		Handler:    handler,
		TLSConfig:  cfg.TLSConfig,
		QUICConfig: cfg.QUICConfig,
	}

	if cfg.HTTP1Addr != "" {
		s.h1 = &http.Server{
			Addr:              cfg.HTTP1Addr,
			Handler:           handler,
			ReadTimeout:       cfg.HTTP1ReadTimeout,
			ReadHeaderTimeout: cfg.HTTP1ReadHeaderTimeout,
			WriteTimeout:      cfg.HTTP1WriteTimeout,
			IdleTimeout:       cfg.HTTP1IdleTimeout,
			MaxHeaderBytes:    cfg.HTTP1MaxHeaderBytes,
		}
	}

	return s
}

// StartHTTP3 starts only the HTTP/3 listener.
func (s *Server) StartHTTP3() error {
	if s.cfg.TLSConfig != nil {
		s.emit("h3", "started", s.cfg.Addr, nil)
		err := s.h3.ListenAndServe()
		return s.handleServeResult("h3", s.cfg.Addr, err)
	}
	if err := ValidateTLSFiles(s.cfg.CertFile, s.cfg.KeyFile); err != nil {
		s.emit("h3", "error", s.cfg.Addr, err)
		return err
	}
	s.emit("h3", "started", s.cfg.Addr, nil)
	err := s.h3.ListenAndServeTLS(s.cfg.CertFile, s.cfg.KeyFile)
	return s.handleServeResult("h3", s.cfg.Addr, err)
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
	if s.cfg.HTTP1DisableKeepAlives {
		s.h1.SetKeepAlivesEnabled(false)
	}
	s.emit("server", "started", s.cfg.Addr, nil)

	errCh := make(chan error, 2)

	go func() {
		s.emit("h1", "started", s.cfg.HTTP1Addr, nil)
		err := s.h1.ListenAndServe()
		errCh <- s.handleServeResult("h1", s.cfg.HTTP1Addr, err)
	}()

	go func() {
		errCh <- s.StartHTTP3()
	}()

	var firstErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
			s.emit("server", "error", s.cfg.Addr, err)
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
			s.emit("h3", "error", s.cfg.Addr, err)
		}
	}

	if s.h1 != nil {
		if err := s.h1.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, err)
			s.emit("h1", "error", s.cfg.HTTP1Addr, err)
		}
	}
	joined := errors.Join(errs...)
	if joined != nil {
		s.emit("server", "error", s.cfg.Addr, joined)
		return joined
	}
	s.emit("server", "stopped", s.cfg.Addr, nil)
	return nil
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

func (s *Server) emit(component, status, addr string, err error) {
	if s.cfg.Logger == nil {
		return
	}
	s.cfg.Logger(Event{
		Time:      time.Now(),
		Component: component,
		Status:    status,
		Addr:      addr,
		Err:       err,
	})
}

func (s *Server) handleServeResult(component, addr string, err error) error {
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		s.emit(component, "stopped", addr, nil)
		return nil
	}
	wrapped := fmt.Errorf("quic: %s server error: %w", component, err)
	s.emit(component, "error", addr, wrapped)
	return wrapped
}
