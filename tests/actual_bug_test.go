package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	nanite "github.com/xDarkicex/nanite"
)

func TestActualMiddlewareBug(t *testing.T) {
	r := nanite.New()

	var executionOrder []string

	// Create middleware that will expose the closure bug
	for i := 0; i < 3; i++ {
		current := i // Capture current value
		r.Use(func(c *nanite.Context, next func()) {
			executionOrder = append(executionOrder, "MW"+strconv.Itoa(current)+"-Enter")
			next()
			executionOrder = append(executionOrder, "MW"+strconv.Itoa(current)+"-Exit")
		})
	}

	r.Get("/test", func(c *nanite.Context) {
		executionOrder = append(executionOrder, "Handler")
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	t.Logf("Execution order: %v", executionOrder)

	// Expected order: MW0-Enter, MW1-Enter, MW2-Enter, Handler, MW2-Exit, MW1-Exit, MW0-Exit
	expected := []string{"MW0-Enter", "MW1-Enter", "MW2-Enter", "Handler", "MW2-Exit", "MW1-Exit", "MW0-Exit"}
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
