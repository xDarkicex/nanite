package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestClosureVariableCaptureBug(t *testing.T) {
	r := nanite.New()

	// This test demonstrates the classic Go closure bug
	// where loop variables are captured by reference

	var results []string

	// Create middleware in a loop - this is where the bug would manifest
	for i := 0; i < 3; i++ {
		r.Use(func(c *nanite.Context, next func()) {
			// If there's a closure bug, all middleware will see the same value of i
			results = append(results, "MW"+strconv.Itoa(i))
			next()
		})
	}

	r.Get("/test", func(c *nanite.Context) {
		results = append(results, "Handler")
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("Results: %v", results)

	// If there's a closure bug, we might see all middleware reporting the same index
	// For example: [MW2, MW2, MW2, Handler] instead of [MW0, MW1, MW2, Handler]
}

func TestRouteMiddlewareClosureBug(t *testing.T) {
	r := nanite.New()

	var results []string

	// Test route-specific middleware with loop variables
	r.Get("/test", func(c *nanite.Context) {
		results = append(results, "Handler")
		c.String(http.StatusOK, "OK")
	}, func(c *nanite.Context, next func()) {
		for i := 0; i < 3; i++ {
			// This inner loop might expose closure issues
			results = append(results, "RouteMW"+strconv.Itoa(i))
		}
		next()
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("Results: %v", results)
}
