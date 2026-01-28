package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestDynamicMiddlewareBehavior(t *testing.T) {
	r := nanite.New()

	// Middleware that should behave differently based on request
	r.Use(func(c *nanite.Context, next func()) {
		// This middleware should only apply to certain paths
		if c.Request.URL.Path == "/admin" {
			c.Set("admin-access", true)
		}
		next()
	})

	r.Use(func(c *nanite.Context, next func()) {
		// Another middleware that depends on previous middleware
		if admin, ok := c.Get("admin-access").(bool); ok && admin {
			c.Set("admin-level", "high")
		}
		next()
	})

	r.Get("/admin", func(c *nanite.Context) {
		adminVal := c.Get("admin-access")
		levelVal := c.Get("admin-level")
		if admin, ok := adminVal.(bool); !ok || !admin {
			t.Errorf("Admin access not set correctly")
		}
		if level, ok := levelVal.(string); !ok || level != "high" {
			t.Errorf("Admin level not set correctly")
		}
		c.String(http.StatusOK, "Admin")
	})

	r.Get("/public", func(c *nanite.Context) {
		// Public route should not have admin context
		if _, ok := c.Get("admin-access"); ok {
			t.Errorf("Admin access should not be set on public route")
		}
		c.String(http.StatusOK, "Public")
	})

	// Test admin route
	req1, _ := http.NewRequest("GET", "/admin", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// Test public route
	req2, _ := http.NewRequest("GET", "/public", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
}

func TestMiddlewareWithRouteSpecificBehavior(t *testing.T) {
	r := nanite.New()

	// Global middleware that should apply to all routes
	r.Use(func(c *nanite.Context, next func()) {
		c.Set("global-middleware", true)
		next()
	})

	// Route-specific middleware
	r.Get("/route1", func(c *nanite.Context) {
		if _, ok := c.Get("global-middleware"); !ok {
			t.Errorf("Global middleware not applied")
		}
		c.String(http.StatusOK, "Route1")
	}, func(c *nanite.Context, next func()) {
		c.Set("route1-specific", true)
		next()
	})

	r.Get("/route2", func(c *nanite.Context) {
		if _, ok := c.Get("global-middleware"); !ok {
			t.Errorf("Global middleware not applied")
		}
		if _, ok := c.Get("route1-specific"); ok {
			t.Errorf("Route1 middleware should not apply to route2")
		}
		c.String(http.StatusOK, "Route2")
	})

	// Test route1
	req1, _ := http.NewRequest("GET", "/route1", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// Test route2
	req2, _ := http.NewRequest("GET", "/route2", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
}

func TestMiddlewareOrderAndDependency(t *testing.T) {
	r := nanite.New()

	var executionOrder []string

	// Middleware that depends on execution order
	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "MW1-Enter")
		c.Set("step", 1)
		next()
		executionOrder = append(executionOrder, "MW1-Exit")
	})

	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "MW2-Enter")
		if step, ok := c.Get("step").(int); ok && step != 1 {
			t.Errorf("Middleware execution order is wrong, expected step 1, got %d", step)
		}
		c.Set("step", 2)
		next()
		executionOrder = append(executionOrder, "MW2-Exit")
	})

	r.Use(func(c *nanite.Context, next func()) {
		executionOrder = append(executionOrder, "MW3-Enter")
		if step, ok := c.Get("step").(int); ok && step != 2 {
			t.Errorf("Middleware execution order is wrong, expected step 2, got %d", step)
		}
		c.Set("step", 3)
		next()
		executionOrder = append(executionOrder, "MW3-Exit")
	})

	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		if step, ok := c.Get("step").(int); ok && step != 3 {
			t.Errorf("Final step should be 3, got %d", step)
		}
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	expected := []string{"MW1-Enter", "MW2-Enter", "MW3-Enter", "Handler", "MW3-Exit", "MW2-Exit", "MW1-Exit"}
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

func TestMiddlewareWithMultipleRoutes(t *testing.T) {
	r := nanite.New()

	var route1Executions, route2Executions []string

	// Shared middleware
	r.Use(func(c *nanite.Context, next func()) {
		if c.Request.URL.Path == "/route1" {
			route1Executions = append(route1Executions, "Shared-Enter")
			next()
			route1Executions = append(route1Executions, "Shared-Exit")
		} else if c.Request.URL.Path == "/route2" {
			route2Executions = append(route2Executions, "Shared-Enter")
			next()
			route2Executions = append(route2Executions, "Shared-Exit")
		}
	})

	r.Get("/route1", func(c *nanite.Context) {
		route1Executions = append(route1Executions, "Handler1")
		c.String(http.StatusOK, "Route1")
	})

	r.Get("/route2", func(c *nanite.Context) {
		route2Executions = append(route2Executions, "Handler2")
		c.String(http.StatusOK, "Route2")
	})

	// Test route1
	req1, _ := http.NewRequest("GET", "/route1", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// Test route2
	req2, _ := http.NewRequest("GET", "/route2", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	// Verify route1 execution
	expected1 := []string{"Shared-Enter", "Handler1", "Shared-Exit"}
	if len(route1Executions) != len(expected1) {
		t.Errorf("Route1: Expected %d executions, got %d: %v", len(expected1), len(route1Executions), route1Executions)
	}

	// Verify route2 execution
	expected2 := []string{"Shared-Enter", "Handler2", "Shared-Exit"}
	if len(route2Executions) != len(expected2) {
		t.Errorf("Route2: Expected %d executions, got %d: %v", len(expected2), len(route2Executions), route2Executions)
	}
}
