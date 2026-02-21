package nanite

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomMethodSuffixRoute(t *testing.T) {
	r := New()
	r.Post("/v1/projects/:name:undelete", func(c *Context) {
		name, ok := c.GetParam("name")
		if !ok {
			t.Fatalf("expected name param")
		}
		c.String(http.StatusOK, name)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj123:undelete", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "proj123" {
		t.Fatalf("expected captured param proj123, got %q", got)
	}
}

func TestCustomMethodSuffixMismatch(t *testing.T) {
	r := New()
	r.Post("/v1/projects/:name:undelete", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj123:archive", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for mismatched suffix, got %d", w.Code)
	}
}

func TestCustomMethodSuffixWithAdditionalPath(t *testing.T) {
	r := New()
	r.Post("/v1/projects/:name:undelete/logs/:logID", func(c *Context) {
		name, _ := c.GetParam("name")
		logID, _ := c.GetParam("logID")
		c.String(http.StatusOK, name+"/"+logID)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/proj123:undelete/logs/55", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "proj123/55" {
		t.Fatalf("expected proj123/55, got %q", got)
	}
}

func TestCustomMethodSuffixRejectsEmptyParam(t *testing.T) {
	r := New()
	r.Post("/v1/projects/:name:undelete", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/:undelete", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty param value, got %d", w.Code)
	}
}

func TestRegularParamStillAllowsColonValues(t *testing.T) {
	r := New()
	r.Get("/users/:id", func(c *Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, id)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/a:b", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "a:b" {
		t.Fatalf("expected id a:b, got %q", got)
	}
}
