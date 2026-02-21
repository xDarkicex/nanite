# Nanite

[![Go Reference](https://pkg.go.dev/badge/github.com/xDarkicex/nanite.svg)](https://pkg.go.dev/github.com/xDarkicex/nanite)
[![Go Report Card](https://goreportcard.com/badge/github.com/xDarkicex/nanite)](https://goreportcard.com/report/github.com/xDarkicex/nanite) 
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

A lightweight, high-performance HTTP router for Go. Designed to be developer-friendly with an Express.js-inspired API, without sacrificing speed.

## Performance

### Microbenchmarks

`go test ./benchmark -benchmem`, Apple M2, Go 1.25.1:

**net/http comparison**

| Framework | Static ns/op | Static allocs/op | Param ns/op | Param allocs/op |
|---|---|---|---|---|
| **Nanite** | **~654** | **0** | **~491** | **0** |
| httprouter | ~159 | 0 | ~259 | 5 |
| chi | ~1136 | 16 | ~1361 | 20 |
| gorilla/mux | ~3452 | 56 | ~3577 | 40 |
| fiber (net/http adaptor) | ~7010 | 152 | ~4910 | 95 |

**Nanite vs Fiber (native, no adaptor)**

| Framework | Static ns/op | Static allocs/op | Param ns/op | Param allocs/op |
|---|---|---|---|---|
| **Nanite** | **~653** | **0** | **~549** | **0** |
| Fiber (native) | ~2220 | 8 | ~1801 | 5 |

Nanite achieves zero allocations on both static and param routes.

### Load Test (wrk)

| Route Type | Throughput | p99 Latency |
|---|---|---|
| Static | 183,120 req/s | 13.72ms |
| Param | 180,619 req/s | 15.20ms |

> Microbenchmarks are in-process. Load tests run against a live server. Fiber adapter numbers include net/http adapter overhead.

## Installation

```bash
go get github.com/xDarkicex/nanite
# Optional websocket module
go get github.com/xDarkicex/nanite/websocket
# Optional QUIC / HTTP/3 module
go get github.com/xDarkicex/nanite/quic
```

## Module Layout

- `github.com/xDarkicex/nanite` — core router/framework (no websocket dependency in core package)
- `github.com/xDarkicex/nanite/websocket` — optional websocket integration module
- `github.com/xDarkicex/nanite/quic` — optional HTTP/3 (QUIC) transport module

This keeps core focused and lets users opt into websocket and HTTP/3 transport features only when needed.

## Quick Start

```go
package main

import (
    "net/http"
    "time"

    "github.com/xDarkicex/nanite"
)

func main() {
    r := nanite.New(
        nanite.WithPanicRecovery(true),
        nanite.WithServerTimeouts(5*time.Second, 60*time.Second, 60*time.Second),
        nanite.WithRouteCacheOptions(1024, 10),
        nanite.WithRouteCacheTuning(4, 8),
    )

    r.Get("/hello", func(c *nanite.Context) {
        c.String(http.StatusOK, "Hello, World!")
    })

    r.Start("8080")
}
```

## Features

- **Radix Tree Routing** — prefix-compressed tree for fast static and dynamic route matching
- **Zero-Allocation Path Parsing** — URL parameters extracted with no heap pressure
- **Request Routing Cache** — frequently accessed routes bypass tree traversal entirely
- **Buffered Response Writer** — optimized I/O with tracked status and pooled buffers
- **Object Pooling** — extensive `sync.Pool` use across contexts, writers, and validation
- **Express-Like Middleware** — global and per-route middleware with `next()` chaining
- **Fluent Middleware API** — chainable middleware composition
- **High-Performance Validation** — pre-allocated error structs, no reflection on the hot path
- **Enhanced Parameter Handling** — complex route parameters with wildcard and typed support
- **Built-in CORS** — preflight short-circuit, returns 204 and halts chain
- **Route Groups** — shared prefix and middleware scoping
- **WebSockets** — optional `nanite/websocket` package
- **Static File Serving** — buffered I/O with efficient path resolution

## Configuration

```go
r := nanite.New(
    nanite.WithPanicRecovery(false),
    nanite.WithRouteCacheOptions(2048, 10),
    nanite.WithRouteCacheTuning(8, 16),
    nanite.WithServerTimeouts(3*time.Second, 30*time.Second, 30*time.Second),
    nanite.WithServerMaxHeaderBytes(1<<20),
    nanite.WithTCPBufferSizes(65536, 65536),
)
```

## Routing

```go
r.Get("/users", listUsers)
r.Post("/users", createUser)
r.Put("/users/:id", updateUser)
r.Delete("/users/:id", deleteUser)

// Route parameters
r.Get("/users/:id", func(c *nanite.Context) {
    id, _ := c.GetParam("id")
    c.JSON(http.StatusOK, map[string]string{"id": id})
})

// Wildcard routes
r.Get("/files/*path", func(c *nanite.Context) {
    path, _ := c.GetParam("path")
    c.JSON(http.StatusOK, map[string]string{"path": path})
})
```

Google-style custom method suffix routes (AIP-136):

```go
r.Post("/v1/projects/:name:undelete", func(c *nanite.Context) {
    name, _ := c.GetParam("name") // "proj123" from "/v1/projects/proj123:undelete"
    c.String(http.StatusOK, name)
})
```

Named routes and reverse routing:

```go
r.NamedGet("users.show", "/users/:id", showUser)
r.NamedGet("files.show", "/files/*path", showFile)

userURL, _ := r.URL("users.show", map[string]string{"id": "42"})
// /users/42

fileURL, _ := r.URL("files.show", map[string]string{"path": "docs/report.pdf"})
// /files/docs/report.pdf

usersURL, _ := r.URLWithQuery(
    "users.show",
    map[string]string{"id": "42"},
    map[string]string{"tab": "activity", "limit": "20"},
)
// /users/42?limit=20&tab=activity
```

## Middleware

```go
// Global
r.Use(LoggerMiddleware)

// Per-route
r.Get("/admin", adminHandler, AuthMiddleware)

// Middleware signature
func LoggerMiddleware(c *nanite.Context, next func()) {
    start := time.Now()
    next()
    fmt.Printf("[%s] %s %dms\n", c.Request.Method, c.Request.URL.Path, time.Since(start).Milliseconds())
}
```

## CORS

```go
r.Use(nanite.CORSMiddleware(&nanite.CORSConfig{
    AllowOrigins:     []string{"https://app.example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
    AllowHeaders:     []string{"Content-Type", "Authorization"},
    ExposeHeaders:    []string{"X-Request-ID"},
    AllowCredentials: true,
    MaxAge:           600,
}))
```

## Security Middleware

Rate limiting (opt-in):

```go
r.Use(nanite.RateLimitMiddleware(&nanite.RateLimitConfig{
    Requests: 120,
    Window:   time.Minute,
}))
```

CSRF protection (opt-in):

```go
r.Use(nanite.CSRFMiddleware(nil))
```

## Validation

```go
emailRule    := nanite.NewValidationChain("email").Required().IsEmail()
passwordRule := nanite.NewValidationChain("password").Required().Length(8, 64)

r.Post("/register", registerHandler, nanite.ValidationMiddleware(emailRule, passwordRule))

func registerHandler(c *nanite.Context) {
    if !c.CheckValidation() {
        return // response already sent
    }
    // ...
}
```

## Route Groups

```go
api := r.Group("/api/v1")
api.Get("/users", listUsers)
api.Post("/users", createUser)

admin := api.Group("/admin", AuthMiddleware)
admin.Get("/stats", getStats)
```

## WebSockets

```go
import (
    nanitews "github.com/xDarkicex/nanite/websocket"
    "github.com/gorilla/websocket"
)

nanitews.Register(r, "/chat", func(conn *websocket.Conn, c *nanite.Context) {
    for {
        mt, msg, err := conn.ReadMessage()
        if err != nil {
            return
        }
        conn.WriteMessage(mt, msg)
    }
})
```

Options-based registration:

```go
nanitews.RegisterWithOptions(r, "/events", handler,
    nanitews.WithAllowedOrigins("https://app.example.com"),
    nanitews.WithUpgrader(&websocket.Upgrader{
        HandshakeTimeout: 5 * time.Second,
    }),
)
```

## HTTP/3 (QUIC)

```go
import (
    "net/http"
    "time"

    "github.com/xDarkicex/nanite"
    nanitequic "github.com/xDarkicex/nanite/quic"
    quicgo "github.com/quic-go/quic-go"
)

r := nanite.New()
r.Get("/healthz", func(c *nanite.Context) {
    c.String(http.StatusOK, "ok")
})

qs := nanitequic.New(r, nanitequic.Config{
    Addr:     ":8443",
    CertFile: "server.crt",
    KeyFile:  "server.key",
    QUICConfig: &quicgo.Config{
        MaxIdleTimeout: 15 * time.Second,
    },
    Logger: func(e nanitequic.Event) {
        // e.Component: h1|h3|server, e.Status: started|stopped|error
        // e.Addr, e.Err, e.Time are available for structured logging.
    },
})

// HTTP/3 only
if err := qs.StartHTTP3(); err != nil {
    panic(err)
}

// Graceful shutdown convenience
if err := qs.ShutdownGraceful(5 * time.Second); err != nil {
    panic(err)
}
```

Router-first helper:

```go
err := nanitequic.StartRouterHTTP3(r, nanitequic.Config{
    Addr:     ":8443",
    CertFile: "server.crt",
    KeyFile:  "server.key",
})
if err != nil {
    panic(err)
}
```

Dual-stack helper (HTTP/1 + HTTP/3):

```go
err := nanitequic.StartRouterDual(r, nanitequic.Config{
    Addr:      ":8443", // UDP (HTTP/3)
    HTTP1Addr: ":8080", // TCP (HTTP/1.1 / HTTP/2)
    CertFile:  "server.crt",
    KeyFile:   "server.key",
    HTTP1ReadHeaderTimeout: 2 * time.Second,
    HTTP1MaxHeaderBytes:    1 << 20,
    HTTP1DisableKeepAlives: false,
})
if err != nil {
    panic(err)
}
```

Full dual-stack example:

```go
package main

import (
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/xDarkicex/nanite"
    nanitequic "github.com/xDarkicex/nanite/quic"
)

func main() {
    r := nanite.New()
    r.Use(nanitequic.AltSvcNaniteMiddleware(nanitequic.AltSvcConfig{
        UDPPort: 8443,
        MaxAge:  300,
    }))
    r.Get("/healthz", func(c *nanite.Context) {
        c.String(http.StatusOK, "ok")
    })

    qs := nanitequic.NewRouterServer(r, nanitequic.Config{
        Addr:                   ":8443", // HTTP/3 over UDP
        HTTP1Addr:              ":8080", // HTTP/1.1+2 over TCP
        CertFile:               "server.crt",
        KeyFile:                "server.key",
        HTTP1ReadHeaderTimeout: 2 * time.Second,
        HTTP1MaxHeaderBytes:    1 << 20,
    })

    go func() {
        if err := qs.StartDualAndServe(); err != nil {
            log.Fatalf("dual-stack server failed: %v", err)
        }
    }()

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    if err := qs.ShutdownGraceful(5 * time.Second); err != nil {
        log.Printf("shutdown error: %v", err)
    }
}
```

Alt-Svc advertisement middleware:

```go
r.Use(nanitequic.AltSvcNaniteMiddleware(nanitequic.AltSvcConfig{
    UDPPort: 8443,
    MaxAge:  86400,
}))
```

TLS helpers:

```go
// Early cert/key path validation
if err := nanitequic.ValidateTLSFiles("server.crt", "server.key"); err != nil {
    panic(err)
}

// Optional: build reusable tls.Config for HTTP/3
tlsCfg, err := nanitequic.LoadTLSConfig("server.crt", "server.key", nil)
if err != nil {
    panic(err)
}

qs := nanitequic.New(r, nanitequic.Config{
    Addr:      ":8443",
    TLSConfig: tlsCfg, // uses ListenAndServe with this config
})
```

Production notes:

- UDP load balancer: HTTP/3 requires UDP routing. Ensure your LB supports UDP and preserves client source address as needed for QUIC behavior.
- Firewall and security groups: open both TCP (for HTTP/1.1+2 fallback) and UDP (for HTTP/3), typically same public port.
- Cert rotation: rotate cert/key atomically on disk and restart gracefully; for advanced rotation, provide `TLSConfig` with dynamic cert loading.
- Alt-Svc rollout: start with low `MaxAge` (for example 60-300s), verify client success/error telemetry, then increase to longer cache durations.
- Mixed clients: keep dual-stack enabled during rollout since some clients or networks still block UDP/QUIC.

## Static Files

```go
r.ServeStatic("/static", "./public")
```

## Context API

```go
// Request
c.Bind(&user)
c.FormValue("name")
c.Query("sort")
c.GetParam("id")
c.File("avatar")

// Response
c.JSON(http.StatusOK, data)
c.String(http.StatusOK, "Hello")
c.HTML(http.StatusOK, "<h1>Hello</h1>")
c.Redirect(http.StatusFound, "/login")
c.SetHeader("X-Custom", "value")
c.Status(http.StatusCreated)
c.Cookie("session", token)

// Scoped data
c.Set("user", user)
if v, ok := c.Get("user"); ok { ... }
c.GetValue("user")
```

## Roadmap

**Completed**
- Radix tree routing
- Zero-allocation path parsing
- Validation system with pre-allocated errors
- Buffered response writer
- Request routing cache
- Hot path allocation reduction
- Fluent middleware API
- Enhanced parameter handling
- Named routes and reverse routing
- CSRF and rate limiting middleware

**Planned**
- Method-not-allowed (405) + Allow header (opt-in)

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/my-feature`)
3. Commit your changes (`git commit -m 'Add my feature'`)
4. Push to the branch (`git push origin feature/my-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
