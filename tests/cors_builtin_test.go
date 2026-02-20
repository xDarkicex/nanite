package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestCORSMiddlewarePreflightShortCircuit(t *testing.T) {
	r := nanite.New()
	called := false
	r.Use(nanite.CORSMiddleware(nil))
	r.Get("/users", func(c *nanite.Context) {
		called = true
		c.String(http.StatusOK, "ok")
	})
	r.Options("/users", func(c *nanite.Context) {
		called = true
		c.String(http.StatusInternalServerError, "should not run")
	})

	req, _ := http.NewRequest(http.MethodOptions, "/users", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if called {
		t.Fatal("handler should not execute for preflight")
	}
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard allow-origin, got %q", got)
	}
}

func TestCORSMiddlewareCustomOrigin(t *testing.T) {
	r := nanite.New()
	r.Use(nanite.CORSMiddleware(&nanite.CORSConfig{
		AllowOrigins:     []string{"https://allowed.example.com"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	r.Get("/users", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	req, _ := http.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("Origin", "https://allowed.example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://allowed.example.com" {
		t.Fatalf("unexpected allow-origin: %q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("expected allow-credentials true, got %q", got)
	}
}
