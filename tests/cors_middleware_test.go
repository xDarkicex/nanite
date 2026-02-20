package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestCORSStyleMiddleware(t *testing.T) {
	r := nanite.New()

	// Simulate CORS middleware that needs to modify response headers
	r.Use(func(c *nanite.Context, next func()) {
		// Set CORS headers before calling next
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		next()

		// CORS middleware shouldn't do anything after next
	})

	// Add another middleware that also modifies headers
	r.Use(func(c *nanite.Context, next func()) {
		c.Writer.Header().Set("X-Custom-Header", "test")
		next()
	})

	r.Get("/test", func(c *nanite.Context) {
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check that headers are set correctly
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("CORS header not set correctly")
	}
	if w.Header().Get("X-Custom-Header") != "test" {
		t.Errorf("Custom header not set correctly")
	}
}

func TestMiddlewareWithContextModification(t *testing.T) {
	r := nanite.New()

	// Middleware that modifies context values
	r.Use(func(c *nanite.Context, next func()) {
		c.Set("middleware1", "value1")
		next()
	})

	// Another middleware that reads and modifies context
	r.Use(func(c *nanite.Context, next func()) {
		raw, _ := c.Get("middleware1")
		val, _ := raw.(string)
		c.Set("middleware2", val+"-modified")
		next()
	})

	r.Get("/test", func(c *nanite.Context) {
		raw1, _ := c.Get("middleware1")
		raw2, _ := c.Get("middleware2")
		val1, _ := raw1.(string)
		val2, _ := raw2.(string)
		if val1 != "value1" || val2 != "value1-modified" {
			t.Errorf("Context values not correct: %s, %s", val1, val2)
		}
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
}

func TestMiddlewareWithEarlyReturn(t *testing.T) {
	r := nanite.New()

	var executionOrder []string

	// Middleware that might return early
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "MW1-Enter")
		// Simulate a condition that might cause early return
		if false { // Change to true to test early return
			c.String(http.StatusForbidden, "Forbidden")
			return
		}
		next()
		executionOrder = append(executionOrder, "MW1-Exit")
	})

	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "MW2-Enter")
		next()
		executionOrder = append(executionOrder, "MW2-Exit")
	})

	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	expected := []string{"MW1-Enter", "MW2-Enter", "Handler", "MW2-Exit", "MW1-Exit"}
	if len(executionOrder) != len(expected) {
		t.Errorf("Expected %d executions, got %d: %v", len(expected), len(executionOrder), executionOrder)
		return
	}

	for i, exp := range expected {
		if executionOrder[i] != exp {
			t.Errorf("At position %d: expected '%s', got '%s'", i, exp, executionOrder[i])
		}
	}
}

func TestMiddlewareWithPanicRecovery(t *testing.T) {
	r := nanite.New()

	var executionOrder []string

	// Middleware with panic recovery
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Recovery-Enter")
		defer func() {
			executionOrder = append(executionOrder, "Recovery-Exit")
			if r := recover(); r != nil {
				c.String(http.StatusInternalServerError, "Recovered from panic")
			}
		}()
		next()
	})

	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Normal-Enter")
		next()
		executionOrder = append(executionOrder, "Normal-Exit")
	})

	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		// Don't panic in this test
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	expected := []string{"Recovery-Enter", "Normal-Enter", "Handler", "Normal-Exit", "Recovery-Exit"}
	if len(executionOrder) != len(expected) {
		t.Errorf("Expected %d executions, got %d: %v", len(expected), len(executionOrder), executionOrder)
		return
	}

	for i, exp := range expected {
		if executionOrder[i] != exp {
			t.Errorf("At position %d: expected '%s', got '%s'", i, exp, executionOrder[i])
		}
	}
}
