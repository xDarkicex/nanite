package quic

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestNewRouterServer(t *testing.T) {
	r := nanite.New()
	s := NewRouterServer(r, Config{Addr: ":0"})
	if s == nil {
		t.Fatal("expected server instance")
	}
}

func TestStartRouterHTTP3ValidatesTLS(t *testing.T) {
	r := nanite.New()
	err := StartRouterHTTP3(r, Config{Addr: ":0"})
	if err == nil {
		t.Fatal("expected TLS validation error")
	}
}

func TestStartRouterDualValidatesHTTP1Addr(t *testing.T) {
	r := nanite.New()
	err := StartRouterDual(r, Config{Addr: ":0", CertFile: "cert.pem", KeyFile: "key.pem"})
	if err == nil {
		t.Fatal("expected HTTP1Addr validation error")
	}
}

func TestAltSvcNaniteMiddleware(t *testing.T) {
	r := nanite.New()
	r.Use(AltSvcNaniteMiddleware(AltSvcConfig{UDPPort: 9443, MaxAge: 600}))
	r.Get("/ok", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Alt-Svc"); got != "h3=\":9443\"; ma=600" {
		t.Fatalf("unexpected Alt-Svc header: %q", got)
	}
}
