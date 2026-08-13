package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/xDarkicex/nanite"
)

func TestNewHub(t *testing.T) {
	h := NewHub(context.Background(), time.Second)
	if h == nil {
		t.Fatal("expected Hub, got nil")
	}
	if h.wheel == nil {
		t.Fatal("expected timing wheel to be initialized")
	}
	h.Stop()
}

func TestHubRegisterUnregister(t *testing.T) {
	h := NewHub(context.Background(), time.Second)
	defer h.Stop()

	conn := &Connection{}
	h.Register(conn)

	h.mu.RLock()
	_, exists := h.connections[conn]
	h.mu.RUnlock()
	if !exists {
		t.Error("expected connection to be registered")
	}

	h.Unregister(conn)
	h.mu.RLock()
	_, exists = h.connections[conn]
	h.mu.RUnlock()
	if exists {
		t.Error("expected connection to be unregistered")
	}
}

func TestHubBroadcast(t *testing.T) {
	h := NewHub(context.Background(), time.Second)
	defer h.Stop()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: rec, Request: req}
	conn := &Connection{ctx: ctx}

	h.Register(conn)
	h.Broadcast([]byte("test"))
	h.Unregister(conn)
}

func TestHubRunStop(t *testing.T) {
	h := NewHub(context.Background(), time.Millisecond)
	time.Sleep(5 * time.Millisecond) // Let it tick
	h.Stop()
	time.Sleep(5 * time.Millisecond) // Let it exit
}

func TestTimingWheel(t *testing.T) {
	tw := newTimingWheel(2 * time.Second)
	if tw.size != 2 {
		t.Errorf("expected size 2, got %d", tw.size)
	}

	conn := &Connection{}
	tw.add(conn)

	if conn.wheelSlot < 0 || conn.wheelSlot >= tw.size {
		t.Errorf("expected valid slot, got %d", conn.wheelSlot)
	}

	tw.remove(conn)
	if conn.wheelSlot != -1 {
		t.Errorf("expected slot -1, got %d", conn.wheelSlot)
	}
}

func TestTimingWheelTick(t *testing.T) {
	tw := newTimingWheel(1 * time.Second)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: rec, Request: req}
	conn := &Connection{ctx: ctx}

	tw.add(conn)
	tw.tick()
}

func TestConnectionStopsAfterRequestCancellation(t *testing.T) {
	reqContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(reqContext)
	ctx := &nanite.Context{Writer: rec, Request: req}
	conn := &Connection{ctx: ctx, wheelSlot: -1}

	cancel()
	if err := conn.SendBytes([]byte("test")); err == nil {
		t.Fatal("expected canceled request to stop SSE writes")
	}
}

func TestHubRemovesConnectionAfterWritePanic(t *testing.T) {
	h := &Hub{
		connections: make(map[*Connection]struct{}),
		wheel:       newTimingWheel(time.Second),
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: panicResponseWriter{}, Request: req}
	conn := &Connection{ctx: ctx, wheelSlot: -1}

	h.Register(conn)
	h.tick()

	h.mu.RLock()
	_, registered := h.connections[conn]
	h.mu.RUnlock()
	if registered {
		t.Fatal("expected a failed SSE write to unregister the connection")
	}
}

func TestHubDoesNotRequeueUnregisteredConnection(t *testing.T) {
	h := &Hub{
		connections: make(map[*Connection]struct{}),
		wheel:       newTimingWheel(time.Second),
	}
	w := newBlockingResponseWriter()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: w, Request: req}
	conn := &Connection{ctx: ctx, wheelSlot: -1}
	h.Register(conn)

	tickDone := make(chan struct{})
	go func() {
		h.tick()
		close(tickDone)
	}()
	<-w.started

	unregisterDone := make(chan struct{})
	go func() {
		h.Unregister(conn)
		close(unregisterDone)
	}()

	deadline := time.After(time.Second)
	for conn.state.Load()&connectionClosed == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out starting connection close")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(w.release)

	select {
	case <-tickDone:
	case <-time.After(time.Second):
		t.Fatal("timing wheel tick did not finish")
	}
	select {
	case <-unregisterDone:
	case <-time.After(time.Second):
		t.Fatal("connection unregister did not finish")
	}

	h.mu.RLock()
	_, registered := h.connections[conn]
	h.mu.RUnlock()
	if registered {
		t.Fatal("connection remained registered after unregister")
	}

	h.wheel.mu.Lock()
	defer h.wheel.mu.Unlock()
	for _, slot := range h.wheel.slots {
		for _, scheduled := range slot {
			if scheduled == conn {
				t.Fatal("unregistered connection was requeued")
			}
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("expected 30s, got %v", cfg.PingInterval)
	}
}

func TestRegister(t *testing.T) {
	r := nanite.New()
	Register(r, "/sse", func(c *Connection, ctx *nanite.Context) {})
}

func TestRegisterWithOptions(t *testing.T) {
	r := nanite.New()
	opt := func(cfg *Config) { cfg.PingInterval = time.Minute }
	RegisterWithOptions(r, "/sse2", func(c *Connection, ctx *nanite.Context) {}, opt)
}

func TestSetupHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.ProtoMajor = 1
	ctx := &nanite.Context{Writer: rec, Request: req}

	setupHeaders(ctx)

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Error("missing Content-Type")
	}
	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Error("missing Cache-Control")
	}
	if rec.Header().Get("Connection") != "keep-alive" {
		t.Error("missing Connection")
	}
}

func TestSendBytes(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: rec, Request: req}

	conn := &Connection{ctx: ctx}
	err := conn.SendBytes([]byte("test"))
	if err != nil {
		t.Fatal(err)
	}
	if rec.Body.String() != "test" {
		t.Errorf("expected 'test', got %s", rec.Body.String())
	}
}

func TestSend(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: rec, Request: req}

	conn := &Connection{ctx: ctx}
	err := conn.Send([]byte("update"), []byte("payload"))
	if err != nil {
		t.Fatal(err)
	}

	expected := "event: update\ndata: payload\n\n"
	if rec.Body.String() != expected {
		t.Errorf("expected %q, got %q", expected, rec.Body.String())
	}
}

func TestPing(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := &nanite.Context{Writer: rec, Request: req}

	conn := &Connection{ctx: ctx}
	err := conn.ping()
	if err != nil {
		t.Fatal(err)
	}

	if rec.Body.String() != string(pingComment) {
		t.Errorf("expected %q, got %q", pingComment, rec.Body.String())
	}
}

func TestStringToBytes(t *testing.T) {
	b := StringToBytes("hello")
	if string(b) != "hello" {
		t.Errorf("expected hello, got %s", string(b))
	}

	empty := StringToBytes("")
	if empty != nil {
		t.Error("expected nil for empty string")
	}
}

type panicResponseWriter struct{}

func (panicResponseWriter) Header() http.Header { return make(http.Header) }

func (panicResponseWriter) WriteHeader(int) {}

func (panicResponseWriter) Write([]byte) (int, error) { panic("write after hijack") }

func (panicResponseWriter) Flush() {}

type blockingResponseWriter struct {
	mu      sync.Once
	started chan struct{}
	release chan struct{}
}

func newBlockingResponseWriter() *blockingResponseWriter {
	return &blockingResponseWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (w *blockingResponseWriter) Header() http.Header { return make(http.Header) }

func (w *blockingResponseWriter) WriteHeader(int) {}

func (w *blockingResponseWriter) Write(p []byte) (int, error) {
	w.mu.Do(func() { close(w.started) })
	<-w.release
	return len(p), nil
}

func (w *blockingResponseWriter) Flush() {}
