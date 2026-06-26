package sse

import (
	"context"
	"net/http"
	"time"
	"unsafe"

	"github.com/xDarkicex/memory"
	"github.com/xDarkicex/nanite"
)

var (
	eventPrefix = []byte("event: ")
	dataPrefix  = []byte("data: ")
	newline     = []byte("\n")
	doubleNL    = []byte("\n\n")
	pingComment = []byte(": ping\n\n")
	okComment   = []byte(": ok\n\n")
)

var fl *memory.FreeList

func init() {
	cfg := memory.DefaultFreeListConfig()
	cfg.SlotSize = 4096
	var err error
	fl, err = memory.NewFreeList(cfg)
	if err != nil {
		panic(err)
	}
}

// Handler is a function that handles SSE connections.
type Handler func(*Connection, *nanite.Context)

// Config controls SSE behavior.
type Config struct {
	PingInterval time.Duration
}

// Option mutates SSE route config.
type Option func(*Config)

// DefaultConfig returns secure SSE defaults.
func DefaultConfig() Config {
	return Config{
		PingInterval: 30 * time.Second,
	}
}

// Connection wraps a Nanite context for SSE operations.
type Connection struct {
	ctx       *nanite.Context
	flusher   http.Flusher
	wheelSlot int
}

// Register mounts an SSE endpoint on a Nanite router.
func Register(r *nanite.Router, path string, handler Handler, middleware ...nanite.MiddlewareFunc) *nanite.Router {
	return RegisterWithConfig(r, path, handler, DefaultConfig(), middleware...)
}

// RegisterWithOptions mounts an SSE endpoint using functional config options.
func RegisterWithOptions(r *nanite.Router, path string, handler Handler, opts ...Option) *nanite.Router {
	cfg := DefaultConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return RegisterWithConfig(r, path, handler, cfg)
}

// RegisterWithConfig mounts an SSE endpoint with explicit config.
func RegisterWithConfig(r *nanite.Router, path string, handler Handler, cfg Config, middleware ...nanite.MiddlewareFunc) *nanite.Router {
	hub := NewHub(context.Background(), cfg.PingInterval)

	wrapped := func(ctx *nanite.Context) {
		setupHeaders(ctx)
		
		flusher, ok := ctx.Writer.(http.Flusher)
		if !ok {
			http.Error(ctx.Writer, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		conn := &Connection{
			ctx:       ctx,
			flusher:   flusher,
			wheelSlot: -1,
		}

		// Proxy breakout flush
		_ = conn.SendBytes(okComment)

		hub.Register(conn)
		defer hub.Unregister(conn)

		handler(conn, ctx)
	}

	r.Get(path, wrapped, middleware...)
	return r
}

func setupHeaders(ctx *nanite.Context) {
	ctx.Writer.Header().Set("Content-Type", "text/event-stream")
	ctx.Writer.Header().Set("Cache-Control", "no-cache")
	if ctx.Request.ProtoMajor == 1 {
		ctx.Writer.Header().Set("Connection", "keep-alive")
	}
	ctx.Writer.WriteHeader(http.StatusOK)
}

// Send formats and sends an SSE message without heap allocations.
func (c *Connection) Send(event []byte, data []byte) error {
	buf, err := fl.Allocate()
	if err != nil {
		return err
	}
	defer func() { _ = fl.Deallocate(buf) }()

	pos := 0
	pos += copy(buf[pos:], eventPrefix)
	pos += copy(buf[pos:], event)
	pos += copy(buf[pos:], newline)
	pos += copy(buf[pos:], dataPrefix)
	pos += copy(buf[pos:], data)
	pos += copy(buf[pos:], doubleNL)

	return c.SendBytes(buf[:pos])
}

// SendBytes sends raw pre-formatted SSE bytes directly.
func (c *Connection) SendBytes(raw []byte) error {
	rc := http.NewResponseController(c.ctx.Writer)
	_ = rc.SetWriteDeadline(time.Now().Add(5 * time.Second))
	
	_, err := c.ctx.Writer.Write(raw)
	if err != nil {
		return err
	}
	
	return rc.Flush()
}

func (c *Connection) ping() error {
	return c.SendBytes(pingComment)
}

// StringToBytes converts a string to a byte slice without allocation.
// The resulting byte slice MUST NOT be modified.
func StringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
