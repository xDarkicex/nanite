package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/gorilla/mux"
	"github.com/julienschmidt/httprouter"
	"github.com/xDarkicex/nanite"
)

// Framework benchmark interface
type framework interface {
	Handle(method, path string)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}

// ============ NANITE ============

type naniteFramework struct {
	router *nanite.Router
}

func newNaniteFramework() *naniteFramework {
	r := nanite.New()
	return &naniteFramework{router: r}
}

func (n *naniteFramework) Handle(method, path string) {
	n.router.Handle(method, path, func(c *nanite.Context) {})
}

func (n *naniteFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.router.ServeHTTP(w, r)
}

// ============ HTTPROUTER ============

type httprouterFramework struct {
	router *httprouter.Router
}

func newHTTPRouterFramework() *httprouterFramework {
	return &httprouterFramework{
		router: httprouter.New(),
	}
}

func (h *httprouterFramework) Handle(method, path string) {
	h.router.Handle(method, path, func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {})
}

func (h *httprouterFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.router.ServeHTTP(w, r)
}

// ============ CHI ============

type chiFramework struct {
	router *chi.Mux
}

func newChiFramework() *chiFramework {
	return &chiFramework{
		router: chi.NewMux(),
	}
}

func (c *chiFramework) Handle(method, path string) {
	c.router.Method(method, path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}

func (c *chiFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.router.ServeHTTP(w, r)
}

// ============ GORILLA MUX ============

type gorillaFramework struct {
	router *mux.Router
}

func newGorillaFramework() *gorillaFramework {
	return &gorillaFramework{
		router: mux.NewRouter(),
	}
}

func (g *gorillaFramework) Handle(method, path string) {
	g.router.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).Methods(method)
}

func (g *gorillaFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.router.ServeHTTP(w, r)
}

// ============ FIBER ============

type fiberFramework struct {
	app         *fiber.App
	httpHandler http.HandlerFunc
}

func newFiberFramework() *fiberFramework {
	app := fiber.New()
	return &fiberFramework{
		app:         app,
		httpHandler: adaptor.FiberApp(app),
	}
}

func (f *fiberFramework) Handle(method, path string) {
	f.app.Add(method, path, func(c *fiber.Ctx) error { return nil })
}

func (f *fiberFramework) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.httpHandler(w, r)
}

// ============ BENCHMARK ROUTES ============

var benchmarkRoutes = []struct {
	method string
	path   string
}{
	{"GET", "/"},
	{"GET", "/users"},
	{"GET", "/users/profile"},
	{"GET", "/products"},
	{"GET", "/products/category"},
	{"POST", "/users"},
	{"PUT", "/users"},
	{"DELETE", "/users"},
}

var paramRoutes = []struct {
	method      string
	patternPath string
	requestPath string
}{
	{"GET", "/users/:id", "/users/123"},
	{"GET", "/products/:id", "/products/456"},
	{"GET", "/categories/:id/products/:pid", "/categories/123/products/456"},
	{"GET", "/users/:id/comments/:cid", "/users/123/comments/789"},
	{"GET", "/users/:id/products/:pid/comments/:cid", "/users/123/products/456/comments/789"},
}

// ============ BENCHMARK FUNCTIONS ============

func benchmarkStaticRoutes(f framework, name string, b *testing.B) {
	// Setup routes
	for _, route := range benchmarkRoutes {
		f.Handle(route.method, route.path)
	}

	b.ReportAllocs()
	b.ResetTimer()
	w := newBenchmarkResponseWriter()
	requests := make([]*http.Request, len(benchmarkRoutes))
	for i, route := range benchmarkRoutes {
		requests[i] = httptest.NewRequest(route.method, route.path, nil)
	}

	for i := 0; i < b.N; i++ {
		for _, req := range requests {
			w.Reset()
			f.ServeHTTP(w, req)
		}
	}
	_ = name
}

func benchmarkParamRoutes(f framework, name string, b *testing.B) {
	// Setup routes
	for _, route := range paramRoutes {
		f.Handle(route.method, normalizeParamPathForFramework(route.patternPath, f))
	}

	b.ReportAllocs()
	b.ResetTimer()
	w := newBenchmarkResponseWriter()
	requests := make([]*http.Request, len(paramRoutes))
	for i, route := range paramRoutes {
		requests[i] = httptest.NewRequest(route.method, route.requestPath, nil)
	}

	for i := 0; i < b.N; i++ {
		for _, req := range requests {
			w.Reset()
			f.ServeHTTP(w, req)
		}
	}
	_ = name
}

func normalizeParamPathForFramework(path string, f framework) string {
	switch f.(type) {
	case *chiFramework, *gorillaFramework:
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts); i++ {
			if strings.HasPrefix(parts[i], ":") && len(parts[i]) > 1 {
				parts[i] = "{" + parts[i][1:] + "}"
			}
		}
		return strings.Join(parts, "/")
	default:
		return path
	}
}

// ============ NANITE BENCHMARKS ============

func BenchmarkNaniteStaticRoutes(b *testing.B) {
	benchmarkStaticRoutes(newNaniteFramework(), "nanite", b)
}

func BenchmarkNaniteParamRoutes(b *testing.B) {
	benchmarkParamRoutes(newNaniteFramework(), "nanite", b)
}

// ============ HTTPROUTER BENCHMARKS ============

func BenchmarkHTTPRouterStaticRoutes(b *testing.B) {
	benchmarkStaticRoutes(newHTTPRouterFramework(), "httprouter", b)
}

func BenchmarkHTTPRouterParamRoutes(b *testing.B) {
	benchmarkParamRoutes(newHTTPRouterFramework(), "httprouter", b)
}

// ============ CHI BENCHMARKS ============

func BenchmarkChiStaticRoutes(b *testing.B) {
	benchmarkStaticRoutes(newChiFramework(), "chi", b)
}

func BenchmarkChiParamRoutes(b *testing.B) {
	benchmarkParamRoutes(newChiFramework(), "chi", b)
}

// ============ GORILLA MUX BENCHMARKS ============

func BenchmarkGorillaStaticRoutes(b *testing.B) {
	benchmarkStaticRoutes(newGorillaFramework(), "gorilla/mux", b)
}

func BenchmarkGorillaParamRoutes(b *testing.B) {
	benchmarkParamRoutes(newGorillaFramework(), "gorilla/mux", b)
}

// ============ FIBER BENCHMARKS ============

func BenchmarkFiberStaticRoutes(b *testing.B) {
	benchmarkStaticRoutes(newFiberFramework(), "fiber", b)
}

func BenchmarkFiberParamRoutes(b *testing.B) {
	benchmarkParamRoutes(newFiberFramework(), "fiber", b)
}

// ============ COMPARISON BENCHMARKS ============

// Run all frameworks in a single benchmark for easy comparison
func BenchmarkComparisonStaticRoutes(b *testing.B) {
	frameworks := []struct {
		name string
		f    framework
	}{
		{name: "nanite", f: newNaniteFramework()},
		{name: "httprouter", f: newHTTPRouterFramework()},
		{name: "chi", f: newChiFramework()},
		{name: "gorilla", f: newGorillaFramework()},
		{name: "fiber", f: newFiberFramework()},
	}

	// Setup routes for all frameworks
	for _, f := range frameworks {
		for _, route := range benchmarkRoutes {
			f.f.Handle(route.method, route.path)
		}
	}

	for _, fw := range frameworks {
		b.Run(fw.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			w := newBenchmarkResponseWriter()
			requests := make([]*http.Request, len(benchmarkRoutes))
			for i, route := range benchmarkRoutes {
				requests[i] = httptest.NewRequest(route.method, route.path, nil)
			}

			for i := 0; i < b.N; i++ {
				for _, req := range requests {
					w.Reset()
					fw.f.ServeHTTP(w, req)
				}
			}
		})
	}
}

func BenchmarkComparisonParamRoutes(b *testing.B) {
	frameworks := []struct {
		name string
		f    framework
	}{
		{name: "nanite", f: newNaniteFramework()},
		{name: "httprouter", f: newHTTPRouterFramework()},
		{name: "chi", f: newChiFramework()},
		{name: "gorilla", f: newGorillaFramework()},
		{name: "fiber", f: newFiberFramework()},
	}

	for _, fw := range frameworks {
		for _, route := range paramRoutes {
			fw.f.Handle(route.method, normalizeParamPathForFramework(route.patternPath, fw.f))
		}
	}

	for _, fw := range frameworks {
		b.Run(fw.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			w := newBenchmarkResponseWriter()
			requests := make([]*http.Request, len(paramRoutes))
			for i, route := range paramRoutes {
				requests[i] = httptest.NewRequest(route.method, route.requestPath, nil)
			}

			for i := 0; i < b.N; i++ {
				for _, req := range requests {
					w.Reset()
					fw.f.ServeHTTP(w, req)
				}
			}
		})
	}
}
