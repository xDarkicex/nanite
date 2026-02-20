# Nanite

[![Go Reference](https://pkg.go.dev/badge/github.com/xDarkicex/nanite.svg)](https://pkg.go.dev/github.com/xDarkicex/nanite)
[![Go Report Card](https://goreportcard.com/badge/github.com/xDarkicex/nanite)](https://goreportcard.com/report/github.com/xDarkicex/nanite) 
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)

Nanite is a lightweight, high-performance HTTP router for Go. It's designed to be developer-friendly, inspired by Express.js, while delivering exceptional performance through advanced optimization techniques.

## Performance

Latest microbench results from this repo (`go test ./benchmark -benchmem`, Apple M2, Go 1.25.1):

### net/http comparison benchmark

| Framework | Static ns/op | Static allocs/op | Param ns/op | Param allocs/op |
|-----------|--------------|------------------|-------------|-----------------|
| Nanite | ~654 | 0 | ~491 | 0 |
| httprouter | ~159 | 0 | ~259 | 5 |
| chi | ~1136 | 16 | ~1361 | 20 |
| gorilla/mux | ~3452 | 56 | ~3577 | 40 |
| fiber (via net/http adaptor) | ~7010 | 152 | ~4910 | 95 |

### Native Nanite vs Native Fiber benchmark

| Framework | Static ns/op | Static allocs/op | Param ns/op | Param allocs/op |
|-----------|--------------|------------------|-------------|-----------------|
| Nanite | ~653 | 0 | ~549 | 0 |
| Fiber (native) | ~2220 | 8 | ~1801 | 5 |

Notes:
- These are in-process microbenchmarks, not external load tests.
- Adapter-based fiber numbers include net/http adapter overhead.

## Features

- 🚀 **Radix Tree Routing**: Advanced prefix compression for faster path matching
- 🧠 **Zero-Allocation Path Parsing**: Parse URL parameters with zero memory overhead
- 🔄 **Buffered Response Writer**: Optimized I/O operations for improved throughput
- 🧩 **Express-Like Middleware**: Intuitive middleware system with support for global and route-specific handlers
- ✅ **Optimized Validation**: High-performance request validation with pre-allocated errors
- 🔌 **WebSockets (Optional Package)**: Use `github.com/xDarkicex/nanite/websocket` for WebSocket routes
- 📁 **Static File Serving**: Efficient static file delivery with buffered I/O
- 🌳 **Route Groups**: Organize routes with shared prefixes and middleware
- 🛡️ **Object Pooling**: Extensive use of sync.Pool to minimize allocations

## Installation

```bash
go get github.com/xDarkicex/nanite
```

## Quick Start

```go
package main

import (
    "fmt"
    "net/http"
    "time"
    
    "github.com/xDarkicex/nanite"
)

func main() {
    // Create a new router with opinionated defaults.
    // All knobs are overridable with With... options.
    r := nanite.New(
        nanite.WithPanicRecovery(true),
        nanite.WithServerTimeouts(5*time.Second, 60*time.Second, 60*time.Second),
        nanite.WithRouteCacheOptions(1024, 10),
        nanite.WithRouteCacheTuning(4, 8),
    )
    
    // Add a simple route
    r.Get("/hello", func(c *nanite.Context) {
        c.String(http.StatusOK, "Hello, World!")
    })
    
    // Start the server
    r.Start("8080")
}
```

## Programmatic Configuration

Nanite exposes functional options for power users while keeping default behavior opinionated.

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

## Routing with Radix Tree

Nanite's radix tree router efficiently handles static and dynamic routes:

```go
// Basic routes
r.Get("/users", listUsers)
r.Post("/users", createUser)
r.Put("/users/:id", updateUser)
r.Delete("/users/:id", deleteUser)

// Route parameters
r.Get("/users/:id", func(c *nanite.Context) {
    id, _ := c.GetParam("id")
    c.JSON(http.StatusOK, map[string]string{
        "id": id,
        "message": "User details",
    })
})

// Wildcard routes
r.Get("/files/*path", func(c *nanite.Context) {
    path, _ := c.GetParam("path")
    c.JSON(http.StatusOK, map[string]string{
        "path": path,
    })
})
```

## Middleware

Middleware functions can be added globally or to specific routes:

```go
// Global middleware
r.Use(LoggerMiddleware)

// Route-specific middleware
r.Get("/admin", adminHandler, AuthMiddleware)

// Middleware function example
func LoggerMiddleware(c *nanite.Context, next func()) {
    // Code executed before the handler
    startTime := time.Now()
    
    // Call the next middleware or handler
    next()
    
    // Code executed after the handler
    duration := time.Since(startTime)
    fmt.Printf("[%s] %s - %dms\n", c.Request.Method, c.Request.URL.Path, duration.Milliseconds())
}
```

## Built-in CORS Middleware

Nanite includes `CORSMiddleware` with proper preflight short-circuit behavior (`OPTIONS` preflight returns `204` and does not continue the chain).

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

## High-Performance Validation

Nanite provides an optimized validation system:

```go
// Create validation rules
emailValidation := nanite.NewValidationChain("email").Required().IsEmail()
passwordValidation := nanite.NewValidationChain("password").Required().Length(8, 64)

// Apply validation middleware
r.Post("/register", registerHandler, nanite.ValidationMiddleware(emailValidation, passwordValidation))

// In your handler, check for validation errors
func registerHandler(c *nanite.Context) {
    if !c.CheckValidation() {
        // Validation failed, response already sent
        return
    }
    
    // Validation passed, continue with registration
    email := c.FormValue("email")
    password := c.FormValue("password")
    // ...
}
```

## WebSockets

WebSocket support is provided by the optional `nanite/websocket` package:

```go
import (
    "github.com/xDarkicex/nanite"
    nanitews "github.com/xDarkicex/nanite/websocket"
    "github.com/gorilla/websocket"
)

nanitews.Register(r, "/chat", func(conn *websocket.Conn, c *nanite.Context) {
    for {
        messageType, p, err := conn.ReadMessage()
        if err != nil {
            return
        }
        if err := conn.WriteMessage(messageType, p); err != nil {
            return
        }
    }
})
```

## Efficient Static File Serving

Serve static files with optimized buffered I/O:

```go
// Serve files from the "public" directory under the "/static" path
r.ServeStatic("/static", "./public")
```

## Context Methods

Nanite provides a rich Context object with many helpful methods:

```go
// Parsing request data
c.Bind(&user)          // Parse JSON request body
c.FormValue("name")    // Get form field
c.Query("sort")        // Get query parameter
c.GetParam("id")       // Get route parameter
c.File("avatar")       // Get uploaded file

// Sending responses
c.JSON(http.StatusOK, data)
c.String(http.StatusOK, "Hello")
c.HTML(http.StatusOK, "Hello")
c.Redirect(http.StatusFound, "/login")

// Managing the response
c.SetHeader("X-Custom", "value")
c.Status(http.StatusCreated)
c.Cookie("session", token)

// Context data
c.Set("user", user)
if v, ok := c.Get("user"); ok {
    _ = v
}
c.GetValue("user") // compatibility one-value accessor
```

## Route Groups

Organize your routes with groups:

```go
// Create an API group
api := r.Group("/api/v1")

// Add routes to the group
api.Get("/users", listUsers)
api.Post("/users", createUser)

// Nested groups with additional middleware
admin := api.Group("/admin", AuthMiddleware)
admin.Get("/stats", getStats)
```

## Upcoming Features

Nanite is actively being enhanced through a 3-phase development plan:

### Phase 1: Core Performance (Completed)
- ✅ Radix tree implementation for optimized routing
- ✅ Validation system optimization with pre-allocated errors
- ✅ Zero-allocation path parsing

### Phase 2: I/O Optimization (In Progress)
- ⚙️ Enhanced buffered response writer
- ⚙️ Request routing cache for frequently accessed routes
- ⚙️ Further reduced allocations in hot paths

### Phase 3: Advanced Features (Planned)
- 🔮 Fluent API for middleware chaining
- 🔮 Named routes and reverse routing for URL generation
- 🔮 Additional built-in security middleware (CSRF, rate limiting, etc.)
- 🔮 Enhanced parameter handling for complex routes
- 🔮 Request rate limiting

## Example Application

Here's a more complete example of a REST API:

```go
package main

import (
    "log"
    "net/http"
    
    "github.com/xDarkicex/nanite"
)

type User struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

func main() {
    r := nanite.New()
    
    // Middleware
    r.Use(LoggerMiddleware)
    
    // Routes
    r.Get("/", func(c *nanite.Context) {
        c.String(http.StatusOK, "API is running")
    })
    
    // API group
    api := r.Group("/api")
    {
        // User validation
        nameValidation := nanite.NewValidationChain("name").Required()
        emailValidation := nanite.NewValidationChain("email").Required().IsEmail()
        
        // User routes
        api.Get("/users", listUsers)
        api.Post("/users", createUser, nanite.ValidationMiddleware(nameValidation, emailValidation))
        api.Get("/users/:id", getUser)
        api.Put("/users/:id", updateUser, nanite.ValidationMiddleware(nameValidation, emailValidation))
        api.Delete("/users/:id", deleteUser)
    }
    
    // Static files
    r.ServeStatic("/assets", "./public")
    
    // Start server
    log.Println("Server started at http://localhost:8080")
    r.Start("8080")
}

func listUsers(c *nanite.Context) {
    users := []User{
        {ID: "1", Name: "Alice", Email: "alice@example.com"},
        {ID: "2", Name: "Bob", Email: "bob@example.com"},
    }
    c.JSON(http.StatusOK, users)
}

func createUser(c *nanite.Context) {
    if !c.CheckValidation() {
        return
    }
    
    var user User
    if err := c.Bind(&user); err != nil {
        c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user data"})
        return
    }
    
    user.ID = "3" // In a real app, generate a unique ID
    c.JSON(http.StatusCreated, user)
}

func getUser(c *nanite.Context) {
    id, _ := c.GetParam("id")
    
    // Simulate database lookup
    user := User{ID: id, Name: "Sample User", Email: "user@example.com"}
    c.JSON(http.StatusOK, user)
}

func updateUser(c *nanite.Context) {
    if !c.CheckValidation() {
        return
    }
    
    id, _ := c.GetParam("id")
    
    var user User
    if err := c.Bind(&user); err != nil {
        c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid user data"})
        return
    }
    
    user.ID = id // Ensure ID matches the route parameter
    c.JSON(http.StatusOK, user)
}

func deleteUser(c *nanite.Context) {
    id, _ := c.GetParam("id")
    c.JSON(http.StatusOK, map[string]string{"message": "User " + id + " deleted"})
}

func LoggerMiddleware(c *nanite.Context, next func()) {
    log.Printf("Request: %s %s", c.Request.Method, c.Request.URL.Path)
    next()
    log.Printf("Response: %d", c.Writer.(*nanite.TrackedResponseWriter).Status())
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the LICENSE file for details.
