package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xDarkicex/nanite"
)

func TestClosureBugExposure(t *testing.T) {
	r := nanite.New()

	var results []string

	// This test tries to expose the closure bug by creating middleware
	// in a way that would cause all middleware to reference the same variable
	for i := 0; i < 3; i++ {
		// Don't capture i in a local variable - this should expose the bug
		r.Use(func(c *nanite.Context, next func()) {
			// If there's a closure bug, all middleware will see the same value of i
			results = append(results, "MW"+string(rune('0'+i))+"-Enter")
			next()
			results = append(results, "MW"+string(rune('0'+i))+"-Exit")
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
	// For example: [MW2-Enter, MW2-Enter, MW2-Enter, Handler, MW2-Exit, MW2-Exit, MW2-Exit]
	// instead of [MW0-Enter, MW1-Enter, MW2-Enter, Handler, MW2-Exit, MW1-Exit, MW0-Exit]

	// Check if all middleware are reporting the same index (which would indicate a bug)
	if len(results) >= 3 {
		firstMW := results[0]
		secondMW := results[1]
		thirdMW := results[2]

		if firstMW == secondMW && secondMW == thirdMW {
			t.Errorf("Closure bug detected! All middleware are reporting the same index: %s", firstMW)
		}
	}
}
