package nanite

import (
	"errors"
	"net/http/httptest"
	"testing"
)

// TestErrorMiddlewareFromRouteSpecificMiddleware tests that error middleware
// is triggered when c.Error() is called from route-specific middleware
func TestErrorMiddlewareFromRouteSpecificMiddleware(t *testing.T) {
	r := New()

	// Track if error middleware was called
	errorMiddlewareCalled := false
	var capturedError error

	// Register error middleware
	r.UseError(func(err error, c *Context, next func()) {
		errorMiddlewareCalled = true
		capturedError = err
		c.JSON(500, map[string]string{"error": err.Error()})
	})

	// Route-specific middleware that calls c.Error()
	sizeLimitMiddleware := func(c *Context, next func()) {
		if c.Request.ContentLength > 1000 {
			c.Error(errors.New("file too large"))
			return
		}
		next()
	}

	// Handler that should not be called if middleware aborts
	handlerCalled := false
	uploadHandler := func(c *Context) {
		handlerCalled = true
		c.JSON(200, map[string]string{"status": "uploaded"})
	}

	// Register route with middleware
	r.Post("/upload", uploadHandler, sizeLimitMiddleware)

	// Create request with large content length
	req := httptest.NewRequest("POST", "/upload", nil)
	req.ContentLength = 2000
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Assertions
	if handlerCalled {
		t.Error("Handler should not be called when middleware aborts")
	}

	if !errorMiddlewareCalled {
		t.Error("Error middleware should be called when c.Error() is invoked from route-specific middleware")
	}

	if capturedError == nil {
		t.Error("Error should be captured by error middleware")
	} else if capturedError.Error() != "file too large" {
		t.Errorf("Expected error 'file too large', got '%s'", capturedError.Error())
	}

	if w.Code != 500 {
		t.Errorf("Expected status 500, got %d", w.Code)
	}

	// Check response body
	expectedBody := `{"error":"file too large"}` + "\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, w.Body.String())
	}
}

// TestErrorMiddlewareFromGlobalMiddleware tests that error middleware
// works correctly when called from global middleware (baseline test)
func TestErrorMiddlewareFromGlobalMiddleware(t *testing.T) {
	r := New()

	errorMiddlewareCalled := false
	var capturedError error

	r.UseError(func(err error, c *Context, next func()) {
		errorMiddlewareCalled = true
		capturedError = err
		c.JSON(500, map[string]string{"error": err.Error()})
	})

	// Global middleware that calls c.Error()
	r.Use(func(c *Context, next func()) {
		if c.Request.Header.Get("X-Fail") == "true" {
			c.Error(errors.New("global middleware error"))
			return
		}
		next()
	})

	handlerCalled := false
	r.Get("/test", func(c *Context) {
		handlerCalled = true
		c.JSON(200, map[string]string{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Fail", "true")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if handlerCalled {
		t.Error("Handler should not be called when global middleware aborts")
	}

	if !errorMiddlewareCalled {
		t.Error("Error middleware should be called when c.Error() is invoked from global middleware")
	}

	if capturedError == nil {
		t.Error("Error should be captured by error middleware")
	}

	if w.Code != 500 {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestErrorMiddlewareFromHandler tests that error middleware
// works correctly when called from the handler itself
func TestErrorMiddlewareFromHandler(t *testing.T) {
	r := New()

	errorMiddlewareCalled := false
	var capturedError error

	r.UseError(func(err error, c *Context, next func()) {
		errorMiddlewareCalled = true
		capturedError = err
		c.JSON(404, map[string]string{"error": err.Error()})
	})

	r.Get("/users/:id", func(c *Context) {
		id := c.MustParam("id")
		if id == "999" {
			c.Error(errors.New("user not found"))
			return
		}
		c.JSON(200, map[string]string{"user": id})
	})

	req := httptest.NewRequest("GET", "/users/999", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if !errorMiddlewareCalled {
		t.Error("Error middleware should be called when c.Error() is invoked from handler")
	}

	if capturedError == nil {
		t.Error("Error should be captured by error middleware")
	} else if capturedError.Error() != "user not found" {
		t.Errorf("Expected error 'user not found', got '%s'", capturedError.Error())
	}

	if w.Code != 404 {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestErrorMiddlewareWithAbortOnly tests that abort without error still returns 404
func TestErrorMiddlewareWithAbortOnly(t *testing.T) {
	r := New()

	errorMiddlewareCalled := false
	r.UseError(func(err error, c *Context, next func()) {
		errorMiddlewareCalled = true
		c.JSON(500, map[string]string{"error": err.Error()})
	})

	// Middleware that aborts without calling c.Error()
	abortMiddleware := func(c *Context, next func()) {
		c.Abort() // Just abort, no error
		return
	}

	handlerCalled := false
	r.Get("/test", func(c *Context) {
		handlerCalled = true
		c.JSON(200, map[string]string{"status": "ok"})
	}, abortMiddleware)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if handlerCalled {
		t.Error("Handler should not be called when middleware aborts")
	}

	if errorMiddlewareCalled {
		t.Error("Error middleware should NOT be called when abort is called without error")
	}

	// Should return 404 when aborted without error
	if w.Code != 404 {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestErrorMiddlewareWithResponseWritten tests that error middleware
// is NOT triggered if a response has already been written
func TestErrorMiddlewareWithResponseWritten(t *testing.T) {
	r := New()

	errorMiddlewareCalled := false
	r.UseError(func(err error, c *Context, next func()) {
		errorMiddlewareCalled = true
		c.JSON(500, map[string]string{"error": err.Error()})
	})

	// Middleware that writes response then calls c.Error()
	middleware := func(c *Context, next func()) {
		c.JSON(400, map[string]string{"error": "custom error"})
		c.Error(errors.New("this error should be ignored"))
		return
	}

	r.Get("/test", func(c *Context) {
		c.JSON(200, map[string]string{"status": "ok"})
	}, middleware)

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if errorMiddlewareCalled {
		t.Error("Error middleware should NOT be called when response is already written")
	}

	// Should keep the original response
	if w.Code != 400 {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	expectedBody := `{"error":"custom error"}` + "\n"
	if w.Body.String() != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, w.Body.String())
	}
}

// TestErrorMiddlewareChaining tests that multiple error handlers can be chained
func TestErrorMiddlewareChaining(t *testing.T) {
	r := New()

	var executionOrder []string

	// First error handler: logging
	r.UseError(func(err error, c *Context, next func()) {
		executionOrder = append(executionOrder, "logger")
		next() // Continue to next error handler
	})

	// Second error handler: send response
	r.UseError(func(err error, c *Context, next func()) {
		executionOrder = append(executionOrder, "responder")
		c.JSON(500, map[string]string{"error": err.Error()})
		next() // Continue (optional)
	})

	// Third error handler: cleanup
	r.UseError(func(err error, c *Context, next func()) {
		executionOrder = append(executionOrder, "cleanup")
	})

	r.Get("/test", func(c *Context) {
		c.Error(errors.New("test error"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Verify execution order
	expectedOrder := []string{"logger", "responder", "cleanup"}
	if len(executionOrder) != len(expectedOrder) {
		t.Errorf("Expected %d error handlers to execute, got %d", len(expectedOrder), len(executionOrder))
	}

	for i, expected := range expectedOrder {
		if i >= len(executionOrder) || executionOrder[i] != expected {
			t.Errorf("Expected handler %d to be '%s', got '%s'", i, expected, executionOrder[i])
		}
	}

	if w.Code != 500 {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}

// TestErrorMiddlewareWithoutNext tests that not calling next() stops the chain
func TestErrorMiddlewareWithoutNext(t *testing.T) {
	r := New()

	var executionOrder []string

	// First error handler: doesn't call next()
	r.UseError(func(err error, c *Context, next func()) {
		executionOrder = append(executionOrder, "first")
		c.JSON(500, map[string]string{"error": err.Error()})
		// Not calling next() - chain should stop here
	})

	// Second error handler: should NOT execute
	r.UseError(func(err error, c *Context, next func()) {
		executionOrder = append(executionOrder, "second")
	})

	r.Get("/test", func(c *Context) {
		c.Error(errors.New("test error"))
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// Only first handler should execute
	if len(executionOrder) != 1 {
		t.Errorf("Expected 1 error handler to execute, got %d", len(executionOrder))
	}

	if len(executionOrder) > 0 && executionOrder[0] != "first" {
		t.Errorf("Expected first handler to execute, got '%s'", executionOrder[0])
	}

	if w.Code != 500 {
		t.Errorf("Expected status 500, got %d", w.Code)
	}
}
