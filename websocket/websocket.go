package websocket

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xDarkicex/nanite"
)

// Handler is a websocket route handler bound to a Nanite request context.
type Handler func(*websocket.Conn, *nanite.Context)

// Config controls websocket upgrade and keepalive behavior.
type Config struct {
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	PingInterval   time.Duration
	MaxMessageSize int64
	BufferSize     int
	AllowedOrigins []string
	Upgrader       *websocket.Upgrader
}

// Option mutates websocket route config.
type Option func(*Config)

// DefaultConfig returns secure websocket defaults.
func DefaultConfig() Config {
	return Config{
		ReadTimeout:    60 * time.Second,
		WriteTimeout:   10 * time.Second,
		PingInterval:   30 * time.Second,
		MaxMessageSize: 1024 * 1024,
		BufferSize:     4096,
	}
}

// WithAllowedOrigins configures websocket origin allow-list patterns.
func WithAllowedOrigins(origins ...string) Option {
	return func(cfg *Config) {
		cfg.AllowedOrigins = append(cfg.AllowedOrigins[:0], origins...)
	}
}

// WithUpgrader injects a preconfigured gorilla upgrader.
func WithUpgrader(upgrader *websocket.Upgrader) Option {
	return func(cfg *Config) {
		cfg.Upgrader = upgrader
	}
}

// NewConfig builds a websocket config from defaults plus functional options.
func NewConfig(opts ...Option) Config {
	cfg := DefaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// Register mounts a websocket endpoint on a Nanite router using GET.
func Register(r *nanite.Router, path string, handler Handler, middleware ...nanite.MiddlewareFunc) *nanite.Router {
	return RegisterWithConfig(r, path, handler, DefaultConfig(), middleware...)
}

// RegisterWithOptions mounts a websocket endpoint using functional config options.
func RegisterWithOptions(r *nanite.Router, path string, handler Handler, opts ...Option) *nanite.Router {
	return RegisterWithConfig(r, path, handler, NewConfig(opts...))
}

// RegisterWithConfig mounts a websocket endpoint with explicit config.
func RegisterWithConfig(r *nanite.Router, path string, handler Handler, cfg Config, middleware ...nanite.MiddlewareFunc) *nanite.Router {
	conf := cfg
	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 60 * time.Second
	}
	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * time.Second
	}
	if conf.PingInterval == 0 {
		conf.PingInterval = 30 * time.Second
	}
	if conf.MaxMessageSize == 0 {
		conf.MaxMessageSize = 1024 * 1024
	}
	if conf.BufferSize == 0 {
		conf.BufferSize = 4096
	}

	upgrader := conf.Upgrader
	if upgrader == nil {
		localConf := conf
		upgrader = &websocket.Upgrader{
			ReadBufferSize:  localConf.BufferSize,
			WriteBufferSize: localConf.BufferSize,
			CheckOrigin: func(req *http.Request) bool {
				return checkOrigin(req, localConf.AllowedOrigins)
			},
		}
	} else {
		if upgrader.ReadBufferSize == 0 {
			upgrader.ReadBufferSize = conf.BufferSize
		}
		if upgrader.WriteBufferSize == 0 {
			upgrader.WriteBufferSize = conf.BufferSize
		}
		if upgrader.CheckOrigin == nil {
			allowed := append([]string(nil), conf.AllowedOrigins...)
			upgrader.CheckOrigin = func(req *http.Request) bool {
				return checkOrigin(req, allowed)
			}
		}
	}

	wrapped := func(ctx *nanite.Context) {
		conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			http.Error(ctx.Writer, "Failed to upgrade to WebSocket", http.StatusBadRequest)
			return
		}

		conn.SetReadLimit(conf.MaxMessageSize)
		wsCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var wg sync.WaitGroup
		cleanup := func() {
			cancel()
			_ = conn.Close()
			wg.Wait()
			ctx.CleanupPooledResources()
		}
		defer cleanup()

		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(conf.ReadTimeout))
			return nil
		})

		wg.Add(1)
		go func() {
			defer wg.Done()
			pingTicker := time.NewTicker(conf.PingInterval)
			defer pingTicker.Stop()

			for {
				select {
				case <-pingTicker.C:
					// WriteControl is safe to call concurrently with application writes.
					// This avoids concurrent-write races between ping loop and handler writes.
					if err := conn.WriteControl(
						websocket.PingMessage,
						nil,
						time.Now().Add(conf.WriteTimeout),
					); err != nil {
						return
					}
				case <-wsCtx.Done():
					return
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Request.Context().Done():
				_ = conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"),
					time.Now().Add(time.Second),
				)
			case <-wsCtx.Done():
			}
		}()

		_ = conn.SetReadDeadline(time.Now().Add(conf.ReadTimeout))
		handler(conn, ctx)

		_ = conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
	}

	r.Get(path, wrapped, middleware...)
	return r
}

func checkOrigin(req *http.Request, allowedOrigins []string) bool {
	origin := req.Header.Get("Origin")
	host := req.Host

	if origin == "" {
		return true
	}

	if len(allowedOrigins) > 0 {
		for _, allowed := range allowedOrigins {
			if originMatches(origin, allowed) {
				return true
			}
		}
		return false
	}

	return origin == "http://"+host || origin == "https://"+host
}

func originMatches(origin, pattern string) bool {
	if strings.Contains(pattern, "*") {
		return matchWildcard(origin, pattern)
	}
	return origin == pattern
}

func matchWildcard(origin, pattern string) bool {
	origins := strings.Split(origin, "://")
	patterns := strings.Split(pattern, "://")

	if len(origins) != 2 || len(patterns) != 2 {
		return false
	}

	if patterns[0] != "*" && origins[0] != patterns[0] {
		return false
	}

	return matchHostWildcard(origins[1], patterns[1])
}

func matchHostWildcard(host, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		mid := strings.TrimSuffix(strings.TrimPrefix(pattern, "*"), "*")
		return strings.Contains(host, mid)
	}

	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(host, strings.TrimPrefix(pattern, "*"))
	}

	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(host, strings.TrimSuffix(pattern, "*"))
	}

	return host == pattern
}
