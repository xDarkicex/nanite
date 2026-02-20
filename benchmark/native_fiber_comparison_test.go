package nanite_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
	"github.com/xDarkicex/nanite"
)

type preparedNanite struct {
	router   *nanite.Router
	writer   *benchmarkResponseWriter
	requests []*http.Request
}

func newPreparedNaniteStatic() *preparedNanite {
	r := nanite.New()
	for _, route := range benchmarkRoutes {
		r.Handle(route.method, route.path, func(c *nanite.Context) {})
	}

	requests := make([]*http.Request, len(benchmarkRoutes))
	for i, route := range benchmarkRoutes {
		requests[i] = httptest.NewRequest(route.method, route.path, nil)
	}

	return &preparedNanite{
		router:   r,
		writer:   newBenchmarkResponseWriter(),
		requests: requests,
	}
}

func newPreparedNaniteParam() *preparedNanite {
	r := nanite.New()
	for _, route := range paramRoutes {
		r.Handle(route.method, route.patternPath, func(c *nanite.Context) {})
	}

	requests := make([]*http.Request, len(paramRoutes))
	for i, route := range paramRoutes {
		requests[i] = httptest.NewRequest(route.method, route.requestPath, nil)
	}

	return &preparedNanite{
		router:   r,
		writer:   newBenchmarkResponseWriter(),
		requests: requests,
	}
}

func (p *preparedNanite) RunIteration() {
	for _, req := range p.requests {
		p.writer.Reset()
		p.router.ServeHTTP(p.writer, req)
	}
}

type fiberPreparedRequest struct {
	request *fasthttp.Request
	ctx     fasthttp.RequestCtx
}

type preparedFiber struct {
	handler  fasthttp.RequestHandler
	requests []*fiberPreparedRequest
}

func newFiberPreparedRequest(method, path string) *fiberPreparedRequest {
	req := fasthttp.AcquireRequest()
	req.Header.SetMethod(method)
	req.SetRequestURI(path)
	return &fiberPreparedRequest{
		request: req,
	}
}

func newPreparedFiberStatic() *preparedFiber {
	app := fiber.New()
	for _, route := range benchmarkRoutes {
		app.Add(route.method, route.path, func(c *fiber.Ctx) error { return nil })
	}

	requests := make([]*fiberPreparedRequest, len(benchmarkRoutes))
	for i, route := range benchmarkRoutes {
		requests[i] = newFiberPreparedRequest(route.method, route.path)
	}

	return &preparedFiber{
		handler:  app.Handler(),
		requests: requests,
	}
}

func newPreparedFiberParam() *preparedFiber {
	app := fiber.New()
	for _, route := range paramRoutes {
		app.Add(route.method, route.patternPath, func(c *fiber.Ctx) error { return nil })
	}

	requests := make([]*fiberPreparedRequest, len(paramRoutes))
	for i, route := range paramRoutes {
		requests[i] = newFiberPreparedRequest(route.method, route.requestPath)
	}

	return &preparedFiber{
		handler:  app.Handler(),
		requests: requests,
	}
}

func (p *preparedFiber) RunIteration() {
	for _, prepared := range p.requests {
		prepared.ctx.Init(prepared.request, nil, nil)
		p.handler(&prepared.ctx)
	}
}

func BenchmarkNativeNaniteStaticRoutes(b *testing.B) {
	prepared := newPreparedNaniteStatic()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		prepared.RunIteration()
	}
}

func BenchmarkNativeNaniteParamRoutes(b *testing.B) {
	prepared := newPreparedNaniteParam()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		prepared.RunIteration()
	}
}

func BenchmarkNativeFiberStaticRoutes(b *testing.B) {
	prepared := newPreparedFiberStatic()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		prepared.RunIteration()
	}
}

func BenchmarkNativeFiberParamRoutes(b *testing.B) {
	prepared := newPreparedFiberParam()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		prepared.RunIteration()
	}
}

func BenchmarkNativeComparisonStaticRoutes(b *testing.B) {
	preparedNanite := newPreparedNaniteStatic()
	preparedFiber := newPreparedFiberStatic()

	b.Run("nanite", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			preparedNanite.RunIteration()
		}
	})

	b.Run("fiber-native", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			preparedFiber.RunIteration()
		}
	})
}

func BenchmarkNativeComparisonParamRoutes(b *testing.B) {
	preparedNanite := newPreparedNaniteParam()
	preparedFiber := newPreparedFiberParam()

	b.Run("nanite", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			preparedNanite.RunIteration()
		}
	})

	b.Run("fiber-native", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			preparedFiber.RunIteration()
		}
	})
}
