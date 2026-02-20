package nanite

import (
	"net/http"
	"testing"
)

func TestNamedRoutesURLGeneration(t *testing.T) {
	r := New()
	r.NamedGet("user.show", "/users/:id", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	url, err := r.URL("user.show", map[string]string{"id": "123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "/users/123" {
		t.Fatalf("expected /users/123, got %s", url)
	}

	method, path, ok := r.NamedRoute("user.show")
	if !ok {
		t.Fatal("expected named route metadata")
	}
	if method != "GET" || path != "/users/:id" {
		t.Fatalf("unexpected metadata method=%s path=%s", method, path)
	}
}

func TestNamedRoutesWildcardURLGeneration(t *testing.T) {
	r := New()
	r.NamedGet("files.show", "/files/*path", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	url, err := r.URL("files.show", map[string]string{"path": "docs/report.pdf"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "/files/docs/report.pdf" {
		t.Fatalf("expected /files/docs/report.pdf, got %s", url)
	}
}

func TestNamedRoutesErrors(t *testing.T) {
	r := New()
	r.NamedGet("user.show", "/users/:id", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	if _, err := r.URL("missing", nil); err == nil {
		t.Fatal("expected missing route error")
	}

	if _, err := r.URL("user.show", nil); err == nil {
		t.Fatal("expected missing param error")
	}
}

func TestNamedRouteDuplicatePanics(t *testing.T) {
	r := New()
	r.NamedGet("dup", "/a", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate name")
		}
	}()

	r.NamedGet("dup", "/b", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})
}

func TestMustURLPanics(t *testing.T) {
	r := New()
	r.NamedGet("user.show", "/users/:id", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for missing params")
		}
	}()
	_ = r.MustURL("user.show", map[string]string{})
}

func TestURLWithQuery(t *testing.T) {
	r := New()
	r.NamedGet("users.show", "/users/:id", func(c *Context) {
		c.String(http.StatusOK, "ok")
	})

	url, err := r.URLWithQuery("users.show", map[string]string{"id": "42"}, map[string]string{
		"sort":  "desc",
		"limit": "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if url != "/users/42?limit=10&sort=desc" {
		t.Fatalf("unexpected url: %s", url)
	}
}

func TestMustURLWithQueryPanics(t *testing.T) {
	r := New()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for missing named route")
		}
	}()
	_ = r.MustURLWithQuery("missing", nil, map[string]string{"a": "b"})
}
