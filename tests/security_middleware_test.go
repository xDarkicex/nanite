package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xDarkicex/nanite"
)

func TestRateLimitMiddleware(t *testing.T) {
	r := nanite.New()
	r.Use(nanite.RateLimitMiddleware(&nanite.RateLimitConfig{
		Requests: 2,
		Window:   time.Minute,
		KeyFunc: func(*nanite.Context) string {
			return "test-client"
		},
	}))

	r.Get("/limited", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if i < 2 && w.Code != http.StatusOK {
			t.Fatalf("request %d expected 200 got %d", i+1, w.Code)
		}
		if i == 2 && w.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d expected 429 got %d", i+1, w.Code)
		}
	}
}

func TestCSRFMiddlewareSetsTokenOnSafeMethod(t *testing.T) {
	r := nanite.New()
	r.Use(nanite.CSRFMiddleware(nil))
	r.Get("/form", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/form", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if token := w.Header().Get("X-CSRF-Token"); token == "" {
		t.Fatal("expected X-CSRF-Token header")
	}
	resp := w.Result()
	defer resp.Body.Close()
	found := false
	for _, c := range resp.Cookies() {
		if c.Name == "nanite_csrf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected CSRF cookie to be set")
	}
}

func TestCSRFMiddlewareBlocksUnsafeWithoutToken(t *testing.T) {
	r := nanite.New()
	r.Use(nanite.CSRFMiddleware(nil))
	r.Post("/submit", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 got %d", w.Code)
	}
}

func TestCSRFMiddlewareAllowsUnsafeWithValidToken(t *testing.T) {
	r := nanite.New()
	r.Use(nanite.CSRFMiddleware(nil))
	r.Get("/form", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.Post("/submit", func(c *nanite.Context) {
		c.String(http.StatusOK, "ok")
	})

	initReq := httptest.NewRequest(http.MethodGet, "/form", nil)
	initW := httptest.NewRecorder()
	r.ServeHTTP(initW, initReq)
	initResp := initW.Result()
	defer initResp.Body.Close()

	var csrfCookie *http.Cookie
	for _, c := range initResp.Cookies() {
		if c.Name == "nanite_csrf" {
			cc := c
			csrfCookie = cc
			break
		}
	}
	if csrfCookie == nil {
		t.Fatal("missing csrf cookie")
	}
	token := initW.Header().Get("X-CSRF-Token")
	if token == "" {
		t.Fatal("missing csrf token header")
	}

	postReq := httptest.NewRequest(http.MethodPost, "/submit", nil)
	postReq.AddCookie(csrfCookie)
	postReq.Header.Set("X-CSRF-Token", token)
	postW := httptest.NewRecorder()
	r.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", postW.Code)
	}
}
