package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	nanite "github.com/xDarkicex/nanite"
)

func TestMiddlewareConcurrencyBug(t *testing.T) {
	r := nanite.New()

	var mu sync.Mutex
	var executionOrder []string

	// Add multiple middleware that might expose concurrency issues
	for i := 0; i < 5; i++ {
		mw := func(c *nanite.Context, next func()) {
			mu.Lock()
			executionOrder = append(executionOrder, "MW"+strconv.Itoa(i)+"-Enter")
			mu.Unlock()

			next()

			mu.Lock()
			executionOrder = append(executionOrder, "MW"+strconv.Itoa(i)+"-Exit")
			mu.Unlock()
		}
		r.Use(mw)
	}

	r.Get("/test", func(c *nanite.Context) {
		mu.Lock()
		executionOrder = append(executionOrder, "Handler")
		mu.Unlock()
		c.String(http.StatusOK, "OK")
	})

	// Make multiple concurrent requests
	var wg sync.WaitGroup
	for j := 0; j < 10; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	// Check that execution order is correct for each request
	// This is hard to verify with concurrent requests, but we can check basic properties
	t.Logf("Total executions: %d", len(executionOrder))
}

func TestMiddlewareStateCaptureBug(t *testing.T) {
	r := nanite.New()

	// This test tries to expose if middleware capture state incorrectly
	var counter int

	// Create middleware that captures the counter value
	for i := 0; i < 3; i++ {
		current := counter // Capture current value
		r.Use(func(c *nanite.Context, next func()) {
			c.Set("mw_value", current)
			next()
		})
		counter++
	}

	r.Get("/test", func(c *nanite.Context) {
		val, ok := c.Get("mw_value").(int)
		if ok {
			t.Logf("Middleware value: %d", val)
		}
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}
}

func TestMiddlewareClosureVariableCapture(t *testing.T) {
	r := nanite.New()

	// Test if variables are captured correctly in closures
	values := []string{"A", "B", "C"}

	for _, val := range values {
		r.Use(func(c *nanite.Context, next func()) {
			// This should capture the current value of 'val'
			c.Set("mw_val", val)
			next()
		})
	}

	r.Get("/test", func(c *nanite.Context) {
		if val, ok := c.Get("mw_val").(string); ok {
			t.Logf("Final middleware value: %s", val)
		}
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", w.Code)
	}
}
