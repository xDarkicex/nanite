package websocket

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/xDarkicex/nanite"
)

func TestNewConfigWithOptions(t *testing.T) {
	upgrader := &websocket.Upgrader{ReadBufferSize: 1234}
	cfg := NewConfig(
		WithAllowedOrigins("https://app.example.com", "https://*.example.com"),
		WithUpgrader(upgrader),
	)

	if cfg.Upgrader != upgrader {
		t.Fatal("expected upgrader option to be applied")
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Fatalf("expected 2 allowed origins, got %d", len(cfg.AllowedOrigins))
	}
	if cfg.AllowedOrigins[0] != "https://app.example.com" {
		t.Fatalf("unexpected first origin: %s", cfg.AllowedOrigins[0])
	}
}

func TestOptionsAffectOriginChecks(t *testing.T) {
	cfg := NewConfig(WithAllowedOrigins("https://allowed.example.com"))

	reqAllowed := httptest.NewRequest("GET", "http://local/ws", nil)
	reqAllowed.Host = "local"
	reqAllowed.Header.Set("Origin", "https://allowed.example.com")
	if !checkOrigin(reqAllowed, cfg.AllowedOrigins) {
		t.Fatal("expected configured origin to pass")
	}

	reqDenied := httptest.NewRequest("GET", "http://local/ws", nil)
	reqDenied.Host = "local"
	reqDenied.Header.Set("Origin", "https://denied.example.com")
	if checkOrigin(reqDenied, cfg.AllowedOrigins) {
		t.Fatal("expected non-configured origin to be denied")
	}
}

func TestRegisterWithConfigDoesNotMutateCallerUpgrader(t *testing.T) {
	r := nanite.New()
	original := &websocket.Upgrader{}

	RegisterWithConfig(r, "/ws", func(*websocket.Conn, *nanite.Context) {}, Config{
		Upgrader: original,
	})

	if original.ReadBufferSize != 0 {
		t.Fatalf("expected caller upgrader ReadBufferSize unchanged, got %d", original.ReadBufferSize)
	}
	if original.WriteBufferSize != 0 {
		t.Fatalf("expected caller upgrader WriteBufferSize unchanged, got %d", original.WriteBufferSize)
	}
	if original.CheckOrigin != nil {
		t.Fatal("expected caller upgrader CheckOrigin to remain nil")
	}
}
