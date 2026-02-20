package quic

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAltSvcHeaderValueDefaults(t *testing.T) {
	cfg := AltSvcConfig{}
	got := cfg.HeaderValue()
	want := "h3=\":8443\"; ma=86400"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAltSvcHeaderValueHelpers(t *testing.T) {
	if got := AltSvcHeaderValueSeconds(9443, 120); got != "h3=\":9443\"; ma=120" {
		t.Fatalf("unexpected seconds value: %q", got)
	}
	if got := AltSvcHeaderValue(8443, 2*time.Minute); got != "h3=\":8443\"; ma=120" {
		t.Fatalf("unexpected duration value: %q", got)
	}
}

func TestAltSvcMiddleware(t *testing.T) {
	mw := AltSvcMiddleware(AltSvcConfig{UDPPort: 9443, MaxAge: 300})
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Alt-Svc"); got != "h3=\":9443\"; ma=300" {
		t.Fatalf("unexpected Alt-Svc header: %q", got)
	}
}
