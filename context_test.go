package nanite

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestContextParams(t *testing.T) {
	r := New()
	handler := func(c *Context) {
		id, exists := c.GetParam("id")
		if !exists {
			t.Errorf("Expected parameter 'id' to exist")
		}
		if id != "123" {
			t.Errorf("Expected parameter 'id' to be '123', got '%s'", id)
		}
		c.JSON(http.StatusOK, map[string]string{"id": id})
	}

	r.Get("/users/:id", handler)

	req, _ := http.NewRequest("GET", "/users/123", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestContextValues(t *testing.T) {
	r := New()
	handler := func(c *Context) {
		c.Values["test"] = "value"
		if c.Values["test"] != "value" {
			t.Errorf("Expected value 'value', got '%v'", c.Values["test"])
		}
		c.JSON(http.StatusOK, map[string]string{"test": "value"})
	}

	r.Get("/test", handler)

	req, _ := http.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestContextAborted(t *testing.T) {
	r := New()

	// Middleware that aborts the request
	middleware := func(c *Context, next func()) {
		c.Abort()
		next()
	}

	handler := func(c *Context) {
		if !c.IsAborted() {
			t.Errorf("Expected context to be aborted")
		}
		c.JSON(http.StatusOK, map[string]string{"message": "should not reach here"})
	}

	r.Use(middleware)
	r.Get("/test", handler)

	req, _ := http.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", rec.Code)
	}
}

func TestContextClearValues(t *testing.T) {
	r := New()
	handler := func(c *Context) {
		c.Values["test"] = "value"
		c.ClearValues()
		if len(c.Values) != 0 {
			t.Errorf("Expected values to be cleared, got %v", c.Values)
		}
		c.JSON(http.StatusOK, map[string]string{"message": "success"})
	}

	r.Get("/test", handler)

	req, _ := http.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}
func TestContextMustParam(t *testing.T) {
	r := New()
	
	// Test 1: MustParam returns value when parameter exists
	handler1 := func(c *Context) {
		id := c.MustParam("id")
		if id != "123" {
			t.Errorf("Expected MustParam('id') to return '123', got '%s'", id)
		}
		c.JSON(http.StatusOK, map[string]string{"id": id})
	}

	r.Get("/users/:id", handler1)

	req1, _ := http.NewRequest("GET", "/users/123", nil)
	rec1 := httptest.NewRecorder()
	r.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec1.Code)
	}

	// Test 2: MustParam panics when parameter is missing
	handler2 := func(c *Context) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected MustParam to panic when parameter is missing")
			} else {
				// Verify panic message contains expected text
				panicMsg := r.(string)
				if !contains(panicMsg, "nanite: required parameter") || !contains(panicMsg, "missing or empty") {
					t.Errorf("Unexpected panic message: %s", panicMsg)
				}
			}
		}()
		
		// This should panic
		_ = c.MustParam("nonexistent")
	}

	r.Get("/test-missing", handler2)

	req2, _ := http.NewRequest("GET", "/test-missing", nil)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	// Test 3: MustParam panics when parameter is empty
	handler3 := func(c *Context) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("Expected MustParam to panic when parameter is empty")
			}
		}()
		
		// This should panic because param exists but is empty
		_ = c.MustParam("empty")
	}

	r.Get("/test-empty/:empty", handler3)

	req3, _ := http.NewRequest("GET", "/test-empty/", nil) // Empty param
	rec3 := httptest.NewRecorder()
	r.ServeHTTP(rec3, req3)
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}


func TestSetCookie(t *testing.T) {
	r := New()
	handler := func(c *Context) {
		c.SetCookie(&http.Cookie{
			Name:     "session",
			Value:    "abc123",
			MaxAge:   3600,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
		})
		c.String(http.StatusOK, "cookie set")
	}

	r.Get("/set-cookie", handler)

	req, _ := http.NewRequest("GET", "/set-cookie", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	// Check that Set-Cookie header was set
	cookies := rec.Header().Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Errorf("Expected Set-Cookie header to be set")
	}

	// Verify cookie properties
	cookieStr := cookies[0]
	if !contains(cookieStr, "session=abc123") {
		t.Errorf("Expected cookie name and value in Set-Cookie header, got: %s", cookieStr)
	}
	if !contains(cookieStr, "HttpOnly") {
		t.Errorf("Expected HttpOnly flag in Set-Cookie header, got: %s", cookieStr)
	}
	if !contains(cookieStr, "Secure") {
		t.Errorf("Expected Secure flag in Set-Cookie header, got: %s", cookieStr)
	}
}
