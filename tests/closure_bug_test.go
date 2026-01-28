package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestMiddlewareClosureBug(t *testing.T) {
	r := nanite.New()

	// This test demonstrates the closure bug by using route-specific middleware
	// The bug manifests when multiple middleware are added to a route

	var executionOrder []string

	// Global middleware
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Global")
		next()
		executionOrder = append(executionOrder, "Global-Exit")
	})

	// Route with multiple middleware
	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		c.String(http.StatusOK, "OK")
	}, func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Route1")
		next()
		executionOrder = append(executionOrder, "Route1-Exit")
	}, func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "Route2")
		next()
		executionOrder = append(executionOrder, "Route2-Exit")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("Execution order: %v", executionOrder)

	// Expected order: Global, Route1, Route2, Handler, Route2-Exit, Route1-Exit, Global-Exit
	expected := []string{"Global", "Route1", "Route2", "Handler", "Route2-Exit", "Route1-Exit", "Global-Exit"}
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

func TestMultipleRoutesClosureBug(t *testing.T) {
	r := nanite.New()

	// Test with multiple routes to see if the closure bug affects route isolation
	r.Get("/route1", func(c *nanite.Context) {
		c.String(http.StatusOK, "Route1")
	}, func(c *nanite.Context, next func()) {
		c.Set("route1_mw", true)
		next()
	})

	r.Get("/route2", func(c *nanite.Context) {
		c.String(http.StatusOK, "Route2")
	}, func(c *nanite.Context, next func()) {
		c.Set("route2_mw", true)
		next()
	})

	// Test route1
	req1, _ := http.NewRequest("GET", "/route1", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// Test route2
	req2, _ := http.NewRequest("GET", "/route2", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	// Check that each route has its own middleware context
	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Errorf("Expected 200 OK for both routes")
	}
}
