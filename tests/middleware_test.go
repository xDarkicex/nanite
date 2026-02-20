package nanite_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xDarkicex/nanite"
)

func trackingMiddleware(id string) nanite.MiddlewareFunc {
	return func(ctx *nanite.Context, next func()) {
		raw, _ := ctx.Get("trace")
		trace, _ := raw.([]string)
		if trace == nil {
			trace = make([]string, 0, 10)
		}
		trace = append(trace, fmt.Sprintf("Enter %s", id))
		ctx.Set("trace", trace)

		next()

		raw, _ = ctx.Get("trace")
		trace, _ = raw.([]string)
		trace = append(trace, fmt.Sprintf("Exit %s", id))
		ctx.Set("trace", trace)
	}
}

func stackTraceMiddleware(ctx *nanite.Context, next func()) {
	var stack [8192]byte
	stackLen := runtime.Stack(stack[:], false)
	raw, _ := ctx.Get("stackDepth")
	trace, _ := raw.([]int)
	if trace == nil {
		trace = make([]int, 0, 10)
	}
	trace = append(trace, stackLen)
	ctx.Set("stackDepth", trace)

	next()
}

func testHandler(ctx *nanite.Context) {
	ctx.String(http.StatusOK, "OK")
}

func simpleHandler(ctx *nanite.Context) {
	raw, _ := ctx.Get("trace")
	trace, _ := raw.([]string)
	if trace == nil {
		trace = []string{}
	}
	trace = append(trace, "Handler")
	ctx.Set("trace", trace)
}

func assertTrace(t *testing.T, ctx *nanite.Context, expected []string) {
	raw, exists := ctx.Get("trace")
	trace, ok := raw.([]string)
	if !exists {
		ok = false
	}
	if !ok {
		trace = nil
	}
	if len(trace) != len(expected) {
		t.Errorf("Trace length expected %d, got %d: %v", len(expected), len(trace), trace)
		return
	}
	for i, exp := range expected {
		if trace[i] != exp {
			t.Errorf("Mismatch at %d: expected '%s', got '%s'", i, exp, trace[i])
			return
		}
	}
}

func TestMiddlewareChainRecursion(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *nanite.Router
	}{
		{
			name: "SingleMiddleware",
			setupFunc: func() *nanite.Router {
				r := nanite.New()
				r.Use(trackingMiddleware("Global"))
				r.Get("/test", testHandler)
				return r
			},
		},
		{
			name: "ThreeGlobalMiddleware",
			setupFunc: func() *nanite.Router {
				r := nanite.New()
				r.Use(trackingMiddleware("Global1"))
				r.Use(trackingMiddleware("Global2"))
				r.Use(trackingMiddleware("Global3"))
				r.Get("/test", testHandler)
				return r
			},
		},
		{
			name: "GlobalPlusRouteMiddleware",
			setupFunc: func() *nanite.Router {
				r := nanite.New()
				r.Use(trackingMiddleware("Global"))
				r.Get("/test", testHandler, trackingMiddleware("Route"))
				return r
			},
		},
		{
			name: "ComplexMiddlewareStack",
			setupFunc: func() *nanite.Router {
				r := nanite.New()
				r.Use(trackingMiddleware("Global1"))
				r.Use(trackingMiddleware("Global2"))
				r.Get("/test", testHandler, trackingMiddleware("Route1"), trackingMiddleware("Route2"), stackTraceMiddleware)
				return r
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := tt.setupFunc()

			for i := 0; i < 1000; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					t.Errorf("Request %d failed with status: %d", i, w.Code)
					break
				}

				if i > 0 && i%100 == 0 {
					var m runtime.MemStats
					runtime.ReadMemStats(&m)
					t.Logf("Request %d - Alloc: %v MB, StackInuse: %v MB", i, m.Alloc/1024/1024, m.StackInuse/1024/1024)
				}
			}
		})
	}
}

func TestRouteWithThreeMiddlewares(t *testing.T) {
	router := nanite.New()
	router.Use(trackingMiddleware("Global"))

	router.Get("/products/:id", testHandler, trackingMiddleware("Route1"), trackingMiddleware("Route2"))

	for i := 0; i < 5000; i++ {
		req := httptest.NewRequest("GET", "/products/456", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if i > 0 && i%1000 == 0 {
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			t.Logf("Iteration %d - Heap: %v MB, Stack: %v MB", i, m.HeapAlloc/1024/1024, m.StackInuse/1024/1024)

			runtime.GC()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestMiddlewareChainingOrder(t *testing.T) {
	r := nanite.New()
	r.Use(trackingMiddleware("Global1"))
	r.Use(trackingMiddleware("Global2"))
	r.Get("/test", simpleHandler, trackingMiddleware("Route1"), trackingMiddleware("Route2"))

	req, _ := http.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	ctx := r.Pool.Get().(*nanite.Context)
	ctx.Reset(w, req)

	handler, params := r.FindHandlerAndMiddleware("GET", "/test")
	var safeParams []nanite.Param
	if len(params) > 0 {
		safeParams = make([]nanite.Param, len(params))
		copy(safeParams, params)
	}
	for i, p := range safeParams {
		if i < len(ctx.Params) {
			ctx.Params[i] = p
		}
	}
	ctx.ParamsCount = len(safeParams)

	handler(ctx)

	expected := []string{"Enter Global1", "Enter Global2", "Enter Route1", "Enter Route2", "Handler", "Exit Route2", "Exit Route1", "Exit Global2", "Exit Global1"}
	assertTrace(t, ctx, expected)

	ctx.CleanupPooledResources()
	r.Pool.Put(ctx)
}

func TestMiddlewareAbortPropagation(t *testing.T) {
	sawLater := false
	sawHandler := false
	r := nanite.New()
	r.Use(func(c *nanite.Context, next func()) {
		c.Abort()
	})
	r.Use(func(c *nanite.Context, next func()) {
		sawLater = true
		next()
	})
	r.Get("/abort", func(c *nanite.Context) {
		sawHandler = true
		c.String(http.StatusOK, "Handler")
	})

	req, _ := http.NewRequest("GET", "/abort", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if sawLater {
		t.Error("Later middleware ran after abort")
	}
	if sawHandler {
		t.Error("Handler ran after abort")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}
}

func TestMiddlewareErrorHandling(t *testing.T) {
	r := nanite.New()
	r.UseError(func(err error, c *nanite.Context, next func()) {
		c.String(http.StatusInternalServerError, fmt.Sprintf("Handled error: %v", err))
	})

	r.Use(func(c *nanite.Context, next func()) {
		panic("test panic")
	})

	r.Get("/error", testHandler)

	req, _ := http.NewRequest("GET", "/error", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "test panic") {
		t.Error("Expected 'test panic' in response")
	}
}

func TestGroupMiddlewareChaining(t *testing.T) {
	r := nanite.New()
	group := r.Group("/api", trackingMiddleware("Group"))
	group.Use(trackingMiddleware("GroupInner"))
	group.Get("/test", simpleHandler, trackingMiddleware("Route"))

	req, _ := http.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	ctx := r.Pool.Get().(*nanite.Context)
	ctx.Reset(w, req)

	handler, params := r.FindHandlerAndMiddleware("GET", "/api/test")
	var safeParams []nanite.Param
	if len(params) > 0 {
		safeParams = make([]nanite.Param, len(params))
		copy(safeParams, params)
	}
	for i, p := range safeParams {
		if i < len(ctx.Params) {
			ctx.Params[i] = p
		}
	}
	ctx.ParamsCount = len(safeParams)

	handler(ctx)

	expected := []string{"Enter Group", "Enter GroupInner", "Enter Route", "Handler", "Exit Route", "Exit GroupInner", "Exit Group"}
	assertTrace(t, ctx, expected)

	ctx.CleanupPooledResources()
	r.Pool.Put(ctx)
}

func TestMiddlewareBlockingBehavior(t *testing.T) {
	const sleepTime = 100 * time.Millisecond
	const totalMin = 250 * time.Millisecond // 3 sleeps

	start := time.Now()
	r := nanite.New()
	r.Use(func(c *nanite.Context, next func()) {
		time.Sleep(sleepTime)
		next()
	})
	r.Use(func(c *nanite.Context, next func()) {
		time.Sleep(sleepTime)
		next()
	})
	r.Get("/block", func(c *nanite.Context) {
		time.Sleep(sleepTime)
		c.String(http.StatusOK, "OK")
	})

	req, _ := http.NewRequest("GET", "/block", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	duration := time.Since(start)
	if duration < totalMin {
		t.Errorf("Not blocking: expected >= %v, got %v", totalMin, duration)
	}
	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestMiddlewareEdgeCases(t *testing.T) {
	t.Run("EmptyChain", func(t *testing.T) {
		r := nanite.New()
		r.Get("/empty", simpleHandler)

		req, _ := http.NewRequest("GET", "/empty", nil)
		w := httptest.NewRecorder()
		ctx := r.Pool.Get().(*nanite.Context)
		ctx.Reset(w, req)

		handler, params := r.FindHandlerAndMiddleware("GET", "/empty")
		var safeParams []nanite.Param
		if len(params) > 0 {
			safeParams = make([]nanite.Param, len(params))
			copy(safeParams, params)
		}
		for i, p := range safeParams {
			if i < len(ctx.Params) {
				ctx.Params[i] = p
			}
		}
		ctx.ParamsCount = len(safeParams)

		handler(ctx)

		expected := []string{"Handler"}
		assertTrace(t, ctx, expected)

		ctx.CleanupPooledResources()
		r.Pool.Put(ctx)
	})

	t.Run("LargeChain", func(t *testing.T) {
		r := nanite.New()
		var mws []nanite.MiddlewareFunc
		for i := 0; i < 15; i++ {
			mws = append(mws, trackingMiddleware(fmt.Sprintf("MW%d", i)))
		}
		r.Use(mws...)
		r.Get("/large", simpleHandler)

		req, _ := http.NewRequest("GET", "/large", nil)
		w := httptest.NewRecorder()
		ctx := r.Pool.Get().(*nanite.Context)
		ctx.Reset(w, req)

		handler, params := r.FindHandlerAndMiddleware("GET", "/large")
		var safeParams []nanite.Param
		if len(params) > 0 {
			safeParams = make([]nanite.Param, len(params))
			copy(safeParams, params)
		}
		for i, p := range safeParams {
			if i < len(ctx.Params) {
				ctx.Params[i] = p
			}
		}
		ctx.ParamsCount = len(safeParams)

		handler(ctx)

		expectedLen := 31 // 15 enter + Handler + 15 exit
		raw, exists := ctx.Get("trace")
		trace, ok := raw.([]string)
		if !exists {
			ok = false
		}
		if !ok || len(trace) != expectedLen {
			t.Errorf("Large chain expected len %d, got %d", expectedLen, len(trace))
		}

		ctx.CleanupPooledResources()
		r.Pool.Put(ctx)
	})
}
