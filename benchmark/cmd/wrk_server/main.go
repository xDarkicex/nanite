package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/mux"
	"github.com/julienschmidt/httprouter"
	"github.com/xDarkicex/nanite"
)

var okBody = []byte("ok")

func main() {
	framework := flag.String("framework", "nanite", "framework to run: nanite|httprouter|chi|gorilla|fiber")
	addr := flag.String("addr", "127.0.0.1:18080", "listen address")
	flag.Parse()
	startPprofIfEnabled()

	switch strings.ToLower(*framework) {
	case "nanite":
		runHTTPServer(*addr, setupNanite())
	case "httprouter":
		runHTTPServer(*addr, setupHTTPRouter())
	case "chi":
		runHTTPServer(*addr, setupChi())
	case "gorilla":
		runHTTPServer(*addr, setupGorilla())
	case "fiber":
		runFiberServer(*addr, setupFiber())
	default:
		log.Fatalf("unknown framework %q", *framework)
	}
}

func startPprofIfEnabled() {
	pprofAddr := os.Getenv("WRK_SERVER_PPROF_ADDR")
	if pprofAddr == "" {
		return
	}
	// Diagnostic mode for live contention/jitter analysis.
	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	go func() {
		log.Printf("starting pprof server on %s", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			log.Printf("pprof server stopped: %v", err)
		}
	}()
}

func runHTTPServer(addr string, handler http.Handler) {
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	log.Printf("starting http server on %s", addr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	waitForSignal()
	_ = srv.Close()
}

func runFiberServer(addr string, app *fiber.App) {
	log.Printf("starting fiber server on %s", addr)
	go func() {
		if err := app.Listen(addr); err != nil {
			log.Fatalf("listen failed: %v", err)
		}
	}()

	waitForSignal()
	_ = app.Shutdown()
}

func waitForSignal() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func setupNanite() http.Handler {
	r := nanite.New()
	if os.Getenv("NANITE_DISABLE_RECOVERY") == "1" {
		r.SetPanicRecovery(false)
	}
	if os.Getenv("NANITE_DISABLE_CACHE") == "1" {
		r.SetRouteCacheOptions(0, 0)
	} else {
		promoteEvery := getenvInt("NANITE_CACHE_PROMOTE_EVERY", 8)
		minDynamic := getenvInt("NANITE_CACHE_MIN_DYNAMIC_ROUTES", 8)
		r.SetRouteCacheTuning(promoteEvery, minDynamic)
	}
	registerHTTPRoutes(func(method, path string, h http.HandlerFunc) {
		r.Handle(method, path, func(c *nanite.Context) { h(c.Writer, c.Request) })
	})
	return r
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func setupHTTPRouter() http.Handler {
	r := httprouter.New()
	registerHTTPRoutes(func(method, path string, h http.HandlerFunc) {
		r.Handle(method, path, func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) { h(w, req) })
	})
	return r
}

func setupChi() http.Handler {
	r := chi.NewMux()
	registerHTTPRoutes(func(method, path string, h http.HandlerFunc) {
		r.Method(method, convertParamSyntax(path, "chi"), h)
	})
	return r
}

func setupGorilla() http.Handler {
	r := mux.NewRouter()
	registerHTTPRoutes(func(method, path string, h http.HandlerFunc) {
		r.Handle(convertParamSyntax(path, "gorilla"), h).Methods(method)
	})
	return r
}

func setupFiber() *fiber.App {
	app := fiber.New()
	registerFiberRoutes(func(method, path string, h fiber.Handler) {
		app.Add(method, path, h)
	})
	return app
}

func registerHTTPRoutes(add func(method, path string, h http.HandlerFunc)) {
	h := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(okBody)
	}

	// Static
	add(http.MethodGet, "/", h)
	add(http.MethodGet, "/users", h)
	add(http.MethodGet, "/users/profile", h)
	add(http.MethodGet, "/products", h)
	add(http.MethodGet, "/products/category", h)
	add(http.MethodPost, "/users", h)
	add(http.MethodPut, "/users", h)
	add(http.MethodDelete, "/users", h)

	// Params
	add(http.MethodGet, "/user/:id", h)
	add(http.MethodGet, "/product/:id", h)
	add(http.MethodGet, "/category/:id/product/:pid", h)
	add(http.MethodGet, "/user/:id/comment/:cid", h)
	add(http.MethodGet, "/user/:id/product/:pid/comment/:cid", h)
}

func registerFiberRoutes(add func(method, path string, h fiber.Handler)) {
	h := func(c *fiber.Ctx) error { return c.Send(okBody) }

	// Static
	add(http.MethodGet, "/", h)
	add(http.MethodGet, "/users", h)
	add(http.MethodGet, "/users/profile", h)
	add(http.MethodGet, "/products", h)
	add(http.MethodGet, "/products/category", h)
	add(http.MethodPost, "/users", h)
	add(http.MethodPut, "/users", h)
	add(http.MethodDelete, "/users", h)

	// Params
	add(http.MethodGet, "/user/:id", h)
	add(http.MethodGet, "/product/:id", h)
	add(http.MethodGet, "/category/:id/product/:pid", h)
	add(http.MethodGet, "/user/:id/comment/:cid", h)
	add(http.MethodGet, "/user/:id/product/:pid/comment/:cid", h)
}

func convertParamSyntax(path, target string) string {
	if target != "chi" && target != "gorilla" {
		return path
	}

	parts := strings.Split(path, "/")
	for i, part := range parts {
		if len(part) > 1 && part[0] == ':' {
			parts[i] = fmt.Sprintf("{%s}", part[1:])
		}
	}
	return strings.Join(parts, "/")
}
