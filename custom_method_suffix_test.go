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

func TestParamSuffixAndPlainParamCoexist(t *testing.T) {
	r := New()
	r.Post("/users/:id:undelete", func(c *Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, "suffix:"+id)
	})
	r.Post("/users/:id", func(c *Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, "plain:"+id)
	})

	req1 := httptest.NewRequest(http.MethodPost, "/users/foo:undelete", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 for suffix route, got %d", w1.Code)
	}
	if got := w1.Body.String(); got != "suffix:foo" {
		t.Fatalf("expected suffix:foo, got %q", got)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/users/foo", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 for plain route, got %d", w2.Code)
	}
	if got := w2.Body.String(); got != "plain:foo" {
		t.Fatalf("expected plain:foo, got %q", got)
	}
}

func TestParamSuffixLongestMatchWins(t *testing.T) {
	r := New()
	r.Post("/items/:id:archive", func(c *Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, "short:"+id)
	})
	r.Post("/items/:id:softarchive", func(c *Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, "long:"+id)
	})

	req := httptest.NewRequest(http.MethodPost, "/items/abc:softarchive", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Body.String(); got != "long:abc" {
		t.Fatalf("expected long:abc, got %q", got)
	}
}
