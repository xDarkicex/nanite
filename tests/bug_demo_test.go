package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestMiddlewareChainingBug(t *testing.T) {
	r := nanite.New()

	// Track execution order
	var executionOrder []string

	// Middleware 1 - should execute first
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Global1")
		next()
		executionOrder = append(executionOrder, "Global1-Exit")
	})

	// Middleware 2 - should execute second
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Global2")
		next()
		executionOrder = append(executionOrder, "Global2-Exit")
	})

	// Route handler
	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Expected order: Global1, Global2, Handler, Global2-Exit, Global1-Exit
	// But due to the bug, we might see incorrect ordering or missing middleware
	t.Logf("Execution order: %v", executionOrder)

	// Check if middleware executed in correct order
	expected := []string{"Global1", "Global2", "Handler", "Global2-Exit", "Global1-Exit"}
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
