# Nanite Framework - Agent Usage Guide

This guide explains how AI agents should use the Nanite HTTP router framework for Go. Nanite is a high-performance router with Express-like ergonomics.

## Quick Start

```go
import "github.com/xDarkicex/nanite"

r := nanite.New()
r.Get("/hello", func(c *nanite.Context) {
    c.String(200, "Hello, World!")
})
r.Start("8080")
```

## Common Patterns

### Basic Server
```go
r := nanite.New(
    nanite.WithPanicRecovery(true),
    nanite.WithServerTimeouts(5*time.Second, 60*time.Second, 60*time.Second),
)

// Add your logging middleware
r.Use(func(c *nanite.Context, next func()) {
    start := time.Now()
    next()
    log.Printf("%s %s - %d (%v)", c.Request.Method, c.Request.URL.Path, 
        c.GetStatus(), time.Since(start))
})

r.Use(nanite.CORSMiddleware(nil))

r.Get("/health", func(c *nanite.Context) {
    c.JSON(200, map[string]string{"status": "ok"})
})

r.Start("8080")
```

### REST API with Groups
```go
api := r.Group("/api/v1")
api.Use(AuthMiddleware)

users := api.Group("/users")
users.Get("/", listUsers)
users.Get("/:id", getUser)
users.Post("/", createUser)
users.Put("/:id", updateUser)
users.Delete("/:id", deleteUser)
```

### Validation Example
```go
emailRule := nanite.NewValidationChain("email").
    Required().
    IsEmail()

passwordRule := nanite.NewValidationChain("password").
    Required().
    Length(8, 128)

r.Post("/register", registerHandler,
    nanite.ValidationMiddleware(emailRule, passwordRule),
)
```

### Error Handling
```go
r.UseError(func(err error, c *nanite.Context, next func()) {
    log.Printf("Error: %v", err)
    c.JSON(500, map[string]string{
        "error": err.Error(),
    })
})

r.Get("/panic", func(c *nanite.Context) {
    panic("oops")
})
```

**Note**: This example returns 500 for all errors (suitable for panics). For production use, inspect the error to return appropriate status codes — see the detailed error inspection pattern in the Error Middleware section below.

## Route Registration

### HTTP Methods
```go
r.Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Post(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Put(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Delete(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Patch(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Options(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Head(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
r.Handle(method string, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
```

All methods return `*Route`, which has a `.Name(string) *Router` method for naming the route. To continue chaining router methods, either call `.Name()` or use separate statements.

### Route Patterns

**Static routes** (O(1) lookup):
```go
r.Get("/users", usersHandler)
```

**Parameterized routes** (Google AIP-136 custom method suffixes supported):
```go
r.Get("/users/:id", userHandler)
r.Get("/posts/:id:undelete", undeleteHandler)  // :undelete suffix
```

**Wildcard routes**:
```go
r.Get("/files/*path", filesHandler)
```

### Route Precedence Rules

Routes are matched in this order of priority:

1. **Exact static routes** - `/users/admin` matches before `/users/:id`
2. **Parameterized routes with suffixes** - `/posts/:id:undelete` (more specific suffix)
3. **Parameterized routes without suffixes** - `/users/:id`
4. **Wildcard routes** - `/files/*path`

**Examples**:
```go
r.Get("/users/admin", adminHandler)      // 1. Exact match for "/users/admin"
r.Get("/users/:id", userHandler)         // 2. Matches "/users/123", not "/users/admin"
r.Get("/posts/:id:undelete", undeleteHandler)  // 3. More specific than plain :id
r.Get("/files/*path", filesHandler)      // 4. Wildcard (lowest priority)
```

**Important**: Register more specific routes before more general ones when they overlap.

### Route Groups

Groups provide path prefixing and middleware scoping. Routes inherit middleware from parent groups and the router.

```go
// Create a group with prefix and middleware
api := r.Group("/api/v1", loggingMiddleware)
api.Get("/users", listUsers)
api.Post("/users", createUser)

// Nested groups inherit parent middleware
admin := api.Group("/admin", authMiddleware)
admin.Get("/stats", getStats)  // Inherits: loggingMiddleware, authMiddleware
```

**Middleware Inheritance**:
- Routes in a group inherit all middleware from:
  1. Global router middleware (via `r.Use()`)
  2. Parent group middleware
  3. Route-specific middleware
- Execution order: Global → Parent Group → Route-specific
- Nested groups inherit from all ancestors

**Example with inheritance chain**:
```go
r.Use(globalMiddleware)                    // Applied to all routes

api := r.Group("/api", apiMiddleware)      // Applied to /api/* routes
api.Use(apiInnerMiddleware)                // Applied to /api/* routes

users := api.Group("/users", usersMiddleware)  // Applied to /api/users/* routes
users.Get("/", listUsers)
```

For the route `users.Get("/", listUsers)`, middleware executes in this order:
1. `globalMiddleware`
2. `apiMiddleware`
3. `apiInnerMiddleware`
4. `usersMiddleware`
5. `listUsers` handler

### Named Routes (Reverse URL Generation)

Named routes allow you to generate URLs from route names instead of hardcoding paths. This makes refactoring easier and reduces errors.

**Basic Usage**:
```go
// Register a named route
r.Get("/users/:id", userHandler).Name("user_show")

// Generate URL from route name
url, err := r.URL("user_show", map[string]string{"id": "123"})
if err != nil {
    // Handle error (e.g., route not found, missing parameters)
}
// Returns: "/users/123"
```

**URL Generation Methods**:
```go
// URL - Returns URL with error handling
url, err := r.URL("user_show", map[string]string{"id": "123"})
// Returns: "/users/123", nil

// MustURL - Panics on error (use for trusted/static paths)
url := r.MustURL("user_show", map[string]string{"id": "123"})
// Panics if route not found or parameters missing

// URLWithQuery - Adds query parameters
url, err := r.URLWithQuery("user_search", 
    map[string]string{"type": "admin"},
    map[string]string{"page": "2", "limit": "10"})
// Returns: "/users/admin?page=2&limit=10", nil

// MustURLWithQuery - Panics on error
url := r.MustURLWithQuery("user_search",
    map[string]string{"type": "admin"},
    map[string]string{"page": "2", "limit": "10"})
// Panics if route not found or parameters missing
```

**Named Routes with Groups**:
```go
api := r.Group("/api/v1")

// Name routes within groups
api.Get("/users/:id", getUser).Name("api.users.show")
api.Post("/users", createUser).Name("api.users.create")
api.Put("/users/:id", updateUser).Name("api.users.update")
api.Delete("/users/:id", deleteUser).Name("api.users.delete")

// Generate URLs
userURL, _ := r.URL("api.users.show", map[string]string{"id": "42"})
// Returns: "/api/v1/users/42"
```

**Wildcard Routes**:
```go
r.Get("/files/*path", serveFile).Name("files.show")

fileURL, _ := r.URL("files.show", map[string]string{"path": "docs/report.pdf"})
// Returns: "/files/docs/report.pdf"
```

**Route Metadata**:
```go
// Retrieve route information
method, path, ok := r.NamedRoute("user_show")
if ok {
    // method = "GET", path = "/users/:id"
}
```

**Method Signatures**:
```go
// Route registration returns *Route for naming
route := r.Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Route
route.Name(name string) *Router  // Assigns a name to the route

// Chaining example
r.Get("/users/:id", handler).Name("user_show")

// Works with all HTTP methods: Get, Post, Put, Delete, Patch, Options, Head, Handle
// Also works with route groups

// URL generation
r.URL(name string, params map[string]string) (string, error)
r.MustURL(name string, params map[string]string) string
r.URLWithQuery(name string, params map[string]string, query map[string]string) (string, error)
r.MustURLWithQuery(name string, params map[string]string, query map[string]string) string

// Route metadata retrieval
r.NamedRoute(name string) (method string, path string, ok bool)
```

**Best Practices**:
- Use descriptive names with dot notation: `"users.show"`, `"api.posts.create"`
- Name routes consistently across your application
- Use `URL()` for dynamic/user-provided data (returns error)
- Use `MustURL()` for static/trusted paths (panics on error)
- Duplicate names will panic at registration time

## Context API

### Request Data Access

```go
// Route parameters
id, ok := c.GetParam("id")                // Optional: returns (string, bool)
id := c.MustParam("id")                   // Required: panics if missing (caught by panic recovery)
id := c.GetIntParam("id")                 // Returns (int, error)
id := c.GetFloatParam("id")               // Returns (float64, error)
b := c.GetBoolParam("id")                 // Returns (bool, error)
u := c.GetUintParam("id")                 // Returns (uint64, error)
s := c.GetStringParamOrDefault("id", "default") // Returns string with default

// Query parameters
q := c.Query("search")                    // Returns string (empty if not found)

// Form data
v := c.FormValue("email")                 // Returns string (empty if not found)

// Files
file, err := c.File("upload")             // Returns (*multipart.FileHeader, error)

// JSON body
var req MyStruct
err := c.Bind(&req)
if err != nil {
    // Returns error if:
    // 1. Content-Type is not application/json
    // 2. JSON is malformed or doesn't match struct types
    c.JSON(400, map[string]string{"error": err.Error()})
    return
}
// Or access via context value set by JSONParsingMiddleware
if raw, exists := c.Get("body"); exists {
    if body, ok := raw.(map[string]interface{}); ok {
        val := body["field"]
    }
}
```

**Parameter Scope**:
- `GetParam()`, `MustParam()`, `GetIntParam()`, `GetFloatParam()`, `GetBoolParam()`, `GetUintParam()`, `GetStringParamOrDefault()` - Access **path parameters only** (captured from route patterns like `:id`)
- `Query()` - Access **query string parameters** (e.g., `?search=term`)
- `FormValue()` - Access **form data** (from POST/PUT body with `application/x-www-form-urlencoded` or `multipart/form-data`)
- `Bind()` - Access **JSON body** (from POST/PUT body with `application/json`)

### Direct HTTP Access

For advanced use cases, you can access the underlying HTTP objects directly:

```go
// Access the raw http.ResponseWriter
c.Writer.Header().Set("X-Custom-Header", "value")
c.Writer.WriteHeader(http.StatusOK)
c.Writer.Write([]byte("raw response"))

// Access the raw *http.Request
method := c.Request.Method
url := c.Request.URL
headers := c.Request.Header
body := c.Request.Body
```

**Note**: When using `c.Writer` directly, you bypass Nanite's response tracking. Use the Context response methods (JSON, String, HTML, etc.) for most cases as they integrate with middleware and error handling.

### Safe Parameter Access Patterns

**Option 1: Check and handle (recommended for user input)**
```go
// For optional parameters or user-provided input
if id, ok := c.GetParam("id"); ok && id != "" {
    // Parameter exists and is not empty
    // Process id...
} else {
    // Handle missing parameter gracefully
    c.JSON(400, map[string]string{"error": "Missing or empty 'id' parameter"})
    return
}
```

**Option 2: MustParam with panic recovery (for trusted/internal paths)**
```go
// For required parameters in trusted code paths
// Enable panic recovery: nanite.New(nanite.WithPanicRecovery(true))
id := c.MustParam("id") // Panics if missing, caught by router's panic recovery
// Continue processing...
```

**Option 3: Type-safe conversion with error handling**
```go
// Convert and validate in one step
id, err := c.GetIntParam("id")
if err != nil {
    c.JSON(400, map[string]string{"error": err.Error()})
    return
}
// id is now a validated integer
```

**Option 4: Default values for optional parameters**
```go
// Use default if parameter is missing or empty
page := c.GetStringParamOrDefault("page", "1")
limit := c.GetStringParamOrDefault("limit", "10")
// Or convert with defaults
pageNum, _ := strconv.Atoi(page) // Safe because page has default
```

**Option 5: Custom validation functions**
```go
func validateNumericID(id string) (int, bool) {
    if id == "" {
        return 0, false
    }
    n, err := strconv.Atoi(id)
    if err != nil || n <= 0 {
        return 0, false
    }
    return n, true
}

// Usage
if id, valid := validateNumericID(c.GetStringParamOrDefault("id", "")); valid {
    // Use validated id
} else {
    c.JSON(400, map[string]string{"error": "Invalid ID"})
    return
}
```

### Bind() Behavior

**Content Types**: Only `application/json` is supported. Other content types will return an error.

**Struct Tags**: Uses Go's standard `encoding/json` package, which respects:
- `json:"field_name"` - Custom JSON field name
- `json:"-"` - Ignore field during marshaling/unmarshaling
- `json:",omitempty"` - Omit empty values
- `json:",string"` - Encode number/boolean as string

**Important**: Nanite's `Bind()` only handles JSON decoding. It does NOT validate structs or check `binding` tags. For validation, use `ValidationMiddleware` or manual validation.

**Error Handling**:
- Returns `error` if JSON decoding fails (malformed JSON, type mismatches)
- Does NOT automatically send error response (handler must handle errors)
- Does NOT validate structs beyond JSON unmarshaling rules
- Does NOT set HTTP status code automatically

**JSONParsingMiddleware + Bind() Interaction**:
- `JSONParsingMiddleware` reads the request body into a buffer and **replaces `ctx.Request.Body` with a buffered reader**, making the body available for downstream handlers
- `Bind()` reads from `ctx.Request.Body` directly
- **Both CAN be safely used on the same route** because the body is buffered and restored:
  - `JSONParsingMiddleware` runs first, buffers the body, stores parsed JSON in context
  - `Bind()` in the handler reads from the restored buffered body
- **Choose one approach per route** (not for safety, but for clarity and performance):
  - **Use `JSONParsingMiddleware` + context access** if you want pre-parsed JSON available to all middleware
  - **Use `Bind()` directly** if you only need JSON in the handler (simpler, no middleware overhead)
  - **Avoid both** if you want to minimize parsing overhead (parsing happens twice)

**Example with manual validation**:
```go
type UserRequest struct {
    Name  string `json:"name"`
    Email string `json:"email"`
    Age   int    `json:"age"`
}

func createUser(c *nanite.Context) {
    var req UserRequest
    if err := c.Bind(&req); err != nil {
        c.JSON(400, map[string]string{"error": "Invalid JSON"})
        return
    }
    
    // Manual validation
    if req.Name == "" {
        c.JSON(400, map[string]string{"error": "Name is required"})
        return
    }
    if req.Age < 18 {
        c.JSON(400, map[string]string{"error": "Must be 18 or older"})
        return
    }
    
    // Process valid request...
}
```

**For automatic validation**, use `ValidationMiddleware` (it handles JSON/form parsing internally):
```go
// Define validation rules
nameRule := nanite.NewValidationChain("name").Required()
emailRule := nanite.NewValidationChain("email").Required().IsEmail()
ageRule := nanite.NewValidationChain("age").Min(18)

// ValidationMiddleware parses JSON/form data and validates — do NOT add JSONParsingMiddleware
r.Post("/users", 
    nanite.ValidationMiddleware(nameRule, emailRule, ageRule),
    createUserHandler
)
```

**Important**: Do NOT use `JSONParsingMiddleware` with `ValidationMiddleware` — ValidationMiddleware handles JSON/form parsing internally. Using both causes double-parsing and wastes resources.

### Response Methods

```go
// JSON response (sets status and writes body)
c.JSON(200, map[string]string{"message": "success"})

// Plain text (sets status and writes body)
c.String(200, "Hello")

// HTML (sets status and writes body)
c.HTML(200, "<h1>Hello</h1>")

// Set status code only (flushes headers with empty body)
c.Status(201)

// Set headers before status (must be before Status() or body methods)
c.SetHeader("X-Custom", "value")
c.Status(201)

// Redirect
c.Redirect(302, "/new-location")

// Cookies (simple, variadic key/value pairs)
c.Cookie("name", "value", "MaxAge", 3600, "Path", "/")

// Cookies (full control with security flags)
c.SetCookie(&http.Cookie{
    Name:     "session",
    Value:    "abc123",
    MaxAge:   3600,
    Path:     "/",
    HttpOnly: true,
    Secure:   true,
    SameSite: http.SameSiteLax,
})
```

**Status Code Behavior**:
- `c.Status(code)` sends a complete response with the given status code and an empty body
- **For responses with a body**: Pass the status directly to `c.JSON()`, `c.String()`, or `c.HTML()` — never call `c.Status()` first
- **For responses with only a status code and no body**: Use `c.Status(code)` alone
- `c.SetHeader()` must be called **before** `c.Status()` or any body-writing method (headers must be set before they're flushed)
- Calling `c.Status()` after a body-writing method is a no-op (headers already flushed)

**Why SetHeader() works with c.JSON() but not c.Status()**:
- `c.JSON()`, `c.String()`, and `c.HTML()` set both headers AND status internally — they flush the complete response
- `c.SetHeader()` only sets response headers; it doesn't flush anything
- Therefore: `c.SetHeader("X-Custom", "value"); c.JSON(201, data)` works because JSON() flushes both headers and status
- But: `c.Status(201); c.JSON(201, data)` fails because Status() already flushed the response

**Correct patterns**:
  - `c.JSON(201, data)` - Status and body in one call (recommended for most cases)
  - `c.String(200, "OK")` - Status and body in one call
  - `c.SetHeader("X-Custom", "value"); c.JSON(201, data)` - Headers, then status and body
  - `c.SetHeader("X-Custom", "value"); c.Status(204)` - Headers, then status-only response (no body)
- **Never do this**: `c.Status(201); c.JSON(201, data)` — Status() sends a complete response; JSON() after it writes to a closed response

**Cookie Methods**:
- `Cookie(name, value, options...)` - Simple variadic API, supports `MaxAge` and `Path` only
  - ⚠️ **Variadic key/value pairs**: Arguments must be even-numbered key/value pairs. Invalid keys or odd arguments are silently ignored.
- `SetCookie(cookie *http.Cookie)` - Full control over all cookie properties (recommended for security-critical cookies)

### Request-Scoped Data

```go
// Store a value in request context
c.Set("user_id", 123)
c.Set("username", "alice")

// Retrieve a value (returns interface{}, bool)
val, ok := c.Get("user_id")
if ok {
    userID := val.(int)  // Type assertion required
    // Use userID...
}

// Clear all values
c.ClearValues()
```

**Method Signatures**:
- `c.Set(key string, value interface{})` - Store any value
- `c.Get(key string) (interface{}, bool)` - Retrieve value; returns (value, found)
- `c.ClearValues()` - Clear all stored values

**Important**: `c.Get()` returns `interface{}`, so you must use type assertion to access the value. The type must match what was stored with `c.Set()`:
```go
// Example 1: Retrieve an int
val, ok := c.Get("user_id")
if ok {
    userID := val.(int)  // Type assertion to int
    // Use userID...
}

// Example 2: Retrieve a string
val, ok := c.Get("username")
if ok {
    username := val.(string)  // Type assertion to string
    // Use username...
}

// Example 3: Retrieve a pointer to custom type
val, ok := c.Get("user")
if ok {
    user := val.(*User)  // Type assertion to *User
    // Use user...
}
```

### Middleware Control

```go
c.Abort()           // Stop middleware chain execution
c.IsAborted()       // Check if request was aborted
c.Error(err)        // Store error and abort (triggers UseError middleware after handler)
c.GetError()        // Retrieve stored error
```

**Request Flow**:
1. **Route lookup**: If no route matches the path/method → NotFoundHandler is called (default: 404)
2. **Middleware chain phase**: Global middleware → Parent group middleware → Route middleware → Handler
   - `c.Abort()` stops the chain immediately (remaining middleware and handler don't execute)
   - `c.Error(err)` stores the error and calls `c.Abort()` to stop the chain
3. **Post-handler phase** (runs after handler completes):
   - If request was aborted AND no response written: 404 Not Found is returned
   - If error is stored AND no response written: UseError middleware runs
   - If response already written: nothing runs (response is final)

**Key points**:
- `c.Abort()` stops the middleware chain. If no response is written before aborting, a 404 is automatically returned.
- `c.Error(err)` stores an error AND aborts; UseError middleware handles the response in the post-handler phase.
- If middleware aborts after writing a response, that response is sent (no 404 override).
- Calling `next()` after `Abort()` is a no-op - the chain will not continue.

**Error Handling Flow**:
- `c.Error(err)` stores the error in context AND calls `c.Abort()` (stops middleware chain)
- After the handler completes, if an error is stored AND no response has been written:
  - `UseError` middleware handlers are triggered (if registered)
  - Otherwise, `Config.ErrorHandler` is called (if set)
- If a response has already been written, error middleware is skipped

**Example - Auth Middleware**:
```go
// Option 1: Write response, then abort
authMiddleware := func(c *nanite.Context, next func()) {
    token := c.Request.Header.Get("Authorization")
    if token == "" {
        c.JSON(401, map[string]string{"error": "Unauthorized"})
        c.Abort()  // Stop chain after writing response
        return
    }
    next()  // Continue to next middleware/handler
}

// Option 2: Abort without response (returns 404)
authMiddleware := func(c *nanite.Context, next func()) {
    token := c.Request.Header.Get("Authorization")
    if token == "" {
        c.Abort()  // Stop chain, 404 will be returned
        return
    }
    next()
}

// Option 3: Use Error() for centralized error handling
authMiddleware := func(c *nanite.Context, next func()) {
    token := c.Request.Header.Get("Authorization")
    if token == "" {
        c.Error(fmt.Errorf("missing authorization header"))  // Stores error, aborts
        return
    }
    next()
}
```

**Example - UseError Middleware**:
```go
r.UseError(func(err error, c *nanite.Context, next func()) {
    log.Printf("Error: %v", err)
    
    // Inspect error to return appropriate status code
    if strings.Contains(err.Error(), "authorization") || strings.Contains(err.Error(), "unauthorized") {
        c.JSON(401, map[string]string{"error": err.Error()})
    } else if strings.Contains(err.Error(), "not found") {
        c.JSON(404, map[string]string{"error": err.Error()})
    } else {
        c.JSON(500, map[string]string{"error": err.Error()})
    }
})

r.Get("/users/:id", func(c *nanite.Context) {
    id := c.MustParam("id")  // Router guarantees :id exists
    
    // Realistic error: database lookup fails
    user, err := db.GetUser(id)
    if err != nil {
        c.Error(fmt.Errorf("user not found: %w", err))  // Stores error, aborts
        return                                           // After handler: UseError middleware runs
    }
    c.JSON(200, user)
})
```

**Production Note**: String matching on error messages is fragile and breaks silently when messages change. For production use, prefer typed errors or error sentinel values:
```go
// Better approach: use typed errors
var ErrUnauthorized = errors.New("unauthorized")
var ErrNotFound = errors.New("not found")

r.UseError(func(err error, c *nanite.Context, next func()) {
    if errors.Is(err, ErrUnauthorized) {
        c.JSON(401, map[string]string{"error": "Unauthorized"})
    } else if errors.Is(err, ErrNotFound) {
        c.JSON(404, map[string]string{"error": "Not found"})
    } else {
        c.JSON(500, map[string]string{"error": "Internal server error"})
    }
})
```

**Important**: Error middleware should inspect the error to return the correct HTTP status code. Different errors warrant different responses:
- Authentication/authorization errors → 401 Unauthorized
- Validation errors → 400 Bad Request
- Resource not found → 404 Not Found
- Server errors → 500 Internal Server Error

**Note**: `c.GetStatus()` reflects what the handler wrote, but UseError only runs when no response has been written, so `GetStatus()` will always return 0 in error middleware. Error middleware must determine the status from the error itself, not from the response status.

```go
type HandlerFunc func(*Context)
type MiddlewareFunc func(*Context, func())
type ErrorMiddlewareFunc func(error, *Context, func())
```

## Middleware System

### Global Middleware
```go
r.Use(loggingMiddleware, authMiddleware)
```

### Per-Route Middleware
```go
r.Get("/admin", adminHandler, requireAdmin)
r.Post("/register", registerHandler,
    nanite.ValidationMiddleware(emailRule, passwordRule),
)
```

**Execution Order**: Route-specific middleware always executes **before** the handler, regardless of parameter position in the call. In the example above:
1. `ValidationMiddleware` runs first (handles JSON/form parsing internally)
2. `registerHandler` runs last (only if middleware doesn't abort)

This is critical for validation: middleware in the variadic position runs **before** the handler, so validation can reject invalid requests before the handler executes.

### Error Middleware

Error middleware runs after the handler completes if an error is stored and no response has been written. Multiple error handlers can be chained.

```go
r.UseError(func(err error, c *nanite.Context, next func()) {
    c.JSON(500, map[string]string{"error": err.Error()})
})
```

**Chaining Multiple Error Handlers**:
Error middleware handlers are executed in registration order. Each handler receives a `next()` function to continue to the next handler.

```go
// First error handler: logging
r.UseError(func(err error, c *nanite.Context, next func()) {
    log.Printf("Error occurred: %v", err)
    next()  // Continue to next error handler
})

// Second error handler: send response
r.UseError(func(err error, c *nanite.Context, next func()) {
    c.JSON(500, map[string]string{"error": err.Error()})
    next()  // Optional: continue to next handler (if any)
})

// Third error handler: cleanup
r.UseError(func(err error, c *nanite.Context, next func()) {
    // Cleanup resources
    next()  // Continue (or omit if this is the last handler)
})
```

**Chaining Rules**:
- Error handlers execute in registration order (first registered runs first)
- Each handler receives the same `error` and `*Context`
- Call `next()` to continue to the next error handler
- If you don't call `next()`, remaining error handlers are **silently skipped**
- Only the first handler to write a response will succeed (subsequent writes are no-ops)
- If no error handler writes a response, `Config.ErrorHandler` is called (if set)

### Built-in Middleware

**CORS**:
```go
// With nil config (uses DefaultCORSConfig())
r.Use(nanite.CORSMiddleware(nil))

// With custom config
r.Use(nanite.CORSMiddleware(&nanite.CORSConfig{
    AllowOrigins: []string{"*"},
    AllowMethods: []string{"GET", "POST"},
    AllowHeaders: []string{"Content-Type"},
    AllowCredentials: false,
    MaxAge: 600,
}))

// DefaultCORSConfig() returns:
// - AllowOrigins: ["*"]
// - AllowMethods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"]
// - AllowHeaders: ["Content-Type", "Authorization"]
// - MaxAge: 600
// - AllowCredentials: false
// - AllowOriginFunc: nil
```

**⚠️ Critical for Authentication APIs**:
- Default config has `AllowCredentials: false` with `AllowOrigins: ["*"]`
- This blocks browsers from sending cookies and Authorization headers in cross-origin requests
- **For APIs using cookies or Authorization headers**, you MUST:
  1. Set `AllowCredentials: true`
  2. Set specific origins (NOT `"*"`): `AllowOrigins: []string{"https://example.com", "https://app.example.com"}`
  3. Browsers reject `AllowCredentials: true` with wildcard origins for security

**Example for auth APIs**:
```go
r.Use(nanite.CORSMiddleware(&nanite.CORSConfig{
    AllowOrigins: []string{"https://example.com"},  // Specific origin, not "*"
    AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders: []string{"Content-Type", "Authorization"},
    AllowCredentials: true,  // Enable for cookies/Authorization headers
    MaxAge: 600,
}))
```

**Rate Limiting**:
```go
r.Use(nanite.RateLimitMiddleware(&nanite.RateLimitConfig{
    Requests: 60,
    Window: time.Minute,
    KeyFunc: func(c *nanite.Context) string {
        return c.Request.Header.Get("X-Forwarded-For")
    },
    StatusCode: 429,
    Message: "Too Many Requests",
}))
```

**CSRF Protection**:
```go
r.Use(nanite.CSRFMiddleware(&nanite.CSRFConfig{
    CookieName: "csrf_token",
    HeaderName: "X-CSRF-Token",
    UnsafeMethods: map[string]struct{}{
        http.MethodPost: {},
        http.MethodPut: {},
        http.MethodPatch: {},
        http.MethodDelete: {},
    },
}))
```

**Validation**:
```go
emailRule := nanite.NewValidationChain("email").Required().IsEmail()
passRule := nanite.NewValidationChain("password").Required().Length(8, 128)
r.Post("/register", handler, nanite.ValidationMiddleware(emailRule, passRule))
```

**Important**: `ValidationMiddleware` validates **request body fields only**:
- **Supported HTTP methods**: POST, PUT, PATCH, DELETE only (GET requests skip validation)
- **Data sources**: Validates fields from request body:
  - JSON body (`application/json`)
  - Form data (`application/x-www-form-urlencoded` or `multipart/form-data`)
- **NOT supported**: Query parameters, path parameters, headers
- For query/path parameter validation, use manual checks in the handler

**JSON Parsing**:
```go
r.Use(nanite.JSONParsingMiddleware(&nanite.JSONParsingConfig{
    MaxSize: 10 << 20,  // 10MB
    TargetKey: "body",
    RequireJSON: false,
}))
```

**When to use**: Use `JSONParsingMiddleware` when you need pre-parsed JSON available across multiple middleware layers or handlers without using `ValidationMiddleware`. The middleware reads the request body into a buffer and stores the parsed JSON in the context under the specified key (default: "body"), making it accessible via `c.Get("body")`.

**Important**: Do NOT use `JSONParsingMiddleware` with `ValidationMiddleware` — ValidationMiddleware handles JSON/form parsing internally. Using both causes double-parsing and wastes resources.

**Use cases**:
- Multiple middleware layers need access to the same parsed JSON
- You want to parse JSON but don't need validation
- You need custom JSON parsing logic before handlers run

## Validation Chain API

```go
// Creation
vc := nanite.NewValidationChain("field_name")

// Rules
vc.Required()
vc.IsEmail()
vc.IsInt()
vc.IsFloat()
vc.IsBoolean()
vc.Length(min, max)
vc.Min(n)
vc.Max(n)
vc.Matches(regexPattern)
vc.OneOf(options...)
vc.IsObject()
vc.IsArray()
vc.Custom(func(value string) error { return nil })
```

**Important**: Do NOT call `.Release()` manually. `ValidationMiddleware` calls it automatically after validation completes. Calling it early will return the chain to the pool and cause errors.

**Validation Scope & Limitations**:
- Validation chains validate **request body fields only** (JSON or form data)
- **Only applies to**: POST, PUT, PATCH, DELETE requests
- **Does NOT validate**: Query parameters, path parameters, headers, cookies
- For query/path parameter validation, use manual checks in handlers:

```go
// Manual validation for query parameters
r.Get("/search", func(c *nanite.Context) {
    query := c.Query("q")
    if query == "" {
        c.JSON(400, map[string]string{"error": "q parameter required"})
        return
    }
    // Process search...
})

// Manual validation for path parameters
r.Get("/users/:id", func(c *nanite.Context) {
    // Validate type conversion (this can actually fail)
    userID, err := c.GetIntParam("id")
    if err != nil {
        c.JSON(400, map[string]string{"error": "User ID must be a valid integer"})
        return
    }
    // Process user with validated userID...
})
```

## Server Lifecycle

### Starting

```go
// HTTP server - binds intelligently based on address format
r.Start("8080")                    // Port only: binds to 0.0.0.0:8080
r.Start("localhost:8080")          // Full address: binds to localhost:8080

// HTTPS server - same address parsing as Start()
r.StartTLS("8443", "cert.pem", "key.pem")              // Binds to 0.0.0.0:8443
r.StartTLS("localhost:8443", "cert.pem", "key.pem")   // Binds to localhost:8443

// HTTP/3 server (using quic module)
s := quic.New(r, quic.Config{Addr: ":8443", CertFile: "cert.pem", KeyFile: "key.pem"})
s.StartHTTP3()
```

**Binding Details**:
- `Start(port)` intelligently parses the address:
  - If the argument contains `:`, it's used as-is (full address)
  - Otherwise, `:` is prepended (port-only format)
- Examples:
  - `Start("8080")` → binds to `0.0.0.0:8080` (all network interfaces)
  - `Start("localhost:8080")` → binds to `localhost:8080` (only local)
  - `Start("127.0.0.1:3000")` → binds to `127.0.0.1:3000` (only loopback)
- The method is **blocking** - it calls `ListenAndServe()` and returns only on error or shutdown
- Returns `nil` on graceful shutdown, or error if binding fails
- Only one server instance can run per router (subsequent `Start()` calls return "server already running" error)

### Shutdown

```go
r.AddShutdownHook(func() error {
    // cleanup
    return nil
})

r.Shutdown(5 * time.Second)        // Graceful with timeout
r.ShutdownImmediate()              // Immediate
```

**Shutdown Details**:
- `Shutdown(timeout)` takes a `time.Duration` (not `context.Context`)
- Internally creates `context.WithTimeout(context.Background(), timeout)` and calls `http.Server.Shutdown(ctx)`
- Executes all registered shutdown hooks before graceful shutdown
- Waits up to `timeout` for active connections to close gracefully
- Returns `nil` on successful shutdown, or error if timeout exceeded or server not running
- `ShutdownImmediate()` calls `http.Server.Close()` for immediate termination (may drop active connections)

### Static Files
```go
r.ServeStatic("/static", "/var/www/files")
```

## Configuration Options

Options are applied in order to `nanite.New()`. **Later options override earlier ones**.

```go
nanite.New(
    nanite.WithPanicRecovery(true),
    nanite.WithRouteCacheOptions(1024, 10),  // size, maxParams
    nanite.WithRouteCacheTuning(4, 8),       // promoteEvery, minDynamicRoutes
    nanite.WithServerTimeouts(read, write, idle),
    nanite.WithServerMaxHeaderBytes(1 << 20),
    nanite.WithTCPBufferSizes(65536, 65536),
    nanite.WithConfig(Config{...}),         // Replaces entire config
    nanite.WithPanicRecovery(false),         // Overrides WithConfig's RecoverPanics
)
```

**Precedence Rules**:
- **Individual `With*` options** modify specific fields
- **`WithConfig()`** replaces the entire config object
- **Later options override earlier ones** (applied sequentially)
- If `WithConfig()` is used, place it **before** individual options you want to override it
- If you want to override `WithConfig()`, place individual options **after** it

**Example - Correct order**:
```go
// ✓ Correct: WithConfig first, then override specific fields
nanite.New(
    nanite.WithConfig(defaultConfig),
    nanite.WithPanicRecovery(false),  // Overrides config's RecoverPanics
)

// ✗ Wrong: Individual options get overridden by WithConfig
nanite.New(
    nanite.WithPanicRecovery(false),
    nanite.WithConfig(defaultConfig),  // Overwrites RecoverPanics back to config's value
)
```

### Config Fields
```go
type Config struct {
    RecoverPanics        bool          // Enable panic recovery middleware
    RouteCacheSize       int           // LRU cache size for dynamic routes (0 to disable)
    RouteMaxParams       int           // Maximum parameters per route (default: 10)
    RouteCachePromote    uint32        // Sampled promotion frequency (power of two mask)
    RouteCacheMinDyn     int           // Minimum dynamic routes before cache activates
    ServerReadTimeout    time.Duration // HTTP read timeout
    ServerWriteTimeout   time.Duration // HTTP write timeout
    ServerIdleTimeout    time.Duration // HTTP idle timeout
    ServerMaxHeaderBytes int           // Maximum request header size
    TCPReadBuffer        int           // TCP read buffer size
    TCPWriteBuffer       int           // TCP write buffer size
    NotFoundHandler      HandlerFunc   // Custom 404 handler (also used for method not allowed)
    ErrorHandler         func(*Context, error) // Custom error handler
}
```

**Error Response Handling**:
- **404 Not Found**: Triggered when no route matches the path. Uses `NotFoundHandler` if set, otherwise returns default "404 page not found" response.
- **405 Method Not Allowed**: Not explicitly handled. If a path exists but the HTTP method doesn't match any registered route, it's treated as 404 (no route found). To implement custom 405 handling, use `NotFoundHandler` and check the request method.
- **500 Internal Server Error**: Triggered by panics (if panic recovery enabled) or errors from `c.Error()`. Uses error middleware or `ErrorHandler` if set.

**Example: Custom 404/405 Handler**:
```go
r := nanite.New(nanite.WithConfig(nanite.Config{
    NotFoundHandler: func(c *nanite.Context) {
        // Note: r is captured by reference in this closure; it's resolved at invocation time
        // after nanite.New() returns, so this pattern is safe and intentional.
        // Check if this is a 405 (method not allowed) or 404 (path not found)
        // by attempting to find a route with a different method
        var methodExists bool
        for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
            if method != c.Request.Method {
                if handler, _ := r.FindHandlerAndMiddleware(method, c.Request.URL.Path); handler != nil {
                    methodExists = true
                    break
                }
            }
        }
        
        if methodExists {
            // Path exists but method doesn't match - 405
            c.JSON(405, map[string]string{
                "error": "Method not allowed",
                "path": c.Request.URL.Path,
                "method": c.Request.Method,
            })
        } else {
            // Path doesn't exist - 404
            c.JSON(404, map[string]string{
                "error": "Not found",
                "path": c.Request.URL.Path,
            })
        }
    },
}))
```

**FindHandlerAndMiddleware** (Advanced):
```go
// Signature
r.FindHandlerAndMiddleware(method string, path string) (HandlerFunc, []Param)

// Returns:
// - HandlerFunc: The handler for the route, or nil if no route matches
// - []Param: Slice of Param structs containing extracted path parameters
//   Each Param has Key (parameter name) and Value (extracted value)
```

**Use Cases**: Advanced route introspection for custom 404/405 handling, route debugging, or conditional routing logic.

**Param Type** (exported from nanite):
```go
type Param struct {
    Key   string // Parameter name from route pattern (e.g., "id" from ":id")
    Value string // Value extracted from request path (raw string, not type-converted)
}
```

### Cache Tuning Methods

```go
// Configure route cache size and max parameters
r.SetRouteCacheOptions(size, maxParams)

// Adjust cache behavior for hit-path contention and usefulness
// promoteEvery: sampled hit promotion frequency (higher = less lock contention)
// minDynamicRoutes: minimum dynamic routes per method before cache is used
r.SetRouteCacheTuning(promoteEvery, minDynamicRoutes)

// Enable/disable panic recovery (disabling removes defer/recover overhead)
r.SetPanicRecovery(enabled)
```

## Utility Methods

```go
// List all registered routes
routes := r.ListRoutes()

// RouteInfo struct definition
type RouteInfo struct {
    Method     string // HTTP method (GET, POST, PUT, DELETE, etc.)
    Path       string // Route path with parameters (e.g., "/users/:id")
    HasHandler bool   // Whether the route has a handler registered
}

// Example: iterate over routes
for _, route := range routes {
    fmt.Printf("%s %s - Handler: %v\n", 
        route.Method, route.Path, route.HasHandler)
}

// Context helpers
c.MustParam("id")           // Returns string (panics if missing)
c.GetError() error          // Returns error or nil
c.IsWritten() bool          // Returns bool (true if response headers sent)
c.WrittenBytes() int64      // Returns int64 (total bytes written to response body)
c.GetStatus() int           // Returns int (HTTP status code, 0 if not set)
```

**Use Cases**:
- `GetStatus()` - Use in logging middleware (after next()) to capture the response status code. Returns 0 if called before a response method has been invoked.
- `IsWritten()` - Use in middleware to avoid writing headers/body twice. Example: logging middleware should only write error responses if no response has been sent yet.
- `WrittenBytes()` - Use for monitoring or debugging to track response size.

## API Quick Reference
- **Creation**: `nanite.New(opts ...Option) *Router`
- **Methods**: Get, Post, Put, Delete, Patch, Options, Head, Handle, Group, Use, UseError, Start, StartTLS, Shutdown, ShutdownImmediate, ServeStatic, AddShutdownHook, SetRouteCacheOptions, SetRouteCacheTuning, SetPanicRecovery, ListRoutes, FindHandlerAndMiddleware
- **Route Naming**: All route registration methods (Get, Post, etc.) return `*Route` which has a `.Name(string)` method for naming routes
- **Named Route Lookup**: NamedRoute
- **URL Generation**: URL, MustURL, URLWithQuery, MustURLWithQuery

### Group
- **Creation**: `router.Group(prefix string, middleware ...MiddlewareFunc) *Group`
- **Methods**: Get, Post, Put, Delete, Patch, Options, Head, Handle, Group, Use (all return `*Route` for naming)

- **Returns**: `*Group` (not `*Router`)
- **Middleware inheritance**: Routes in a group inherit all middleware from parent group and router

### quic.Server (HTTP/3 module)
- **Creation**: `quic.New(handler http.Handler, cfg Config) *Server`
- **Methods**: StartHTTP3, StartDualAndServe, Shutdown, ShutdownGraceful

### Context
- **Handler signature**: `func(*Context)`
- **Direct access**: Writer (http.ResponseWriter), Request (*http.Request)
- **Access request data**: GetParam, MustParam, GetIntParam, GetFloatParam, GetBoolParam, GetUintParam, GetStringParamOrDefault, Query, FormValue, File, Bind
- **Send responses**: JSON, String, HTML, SetHeader, Status, Redirect, Cookie, SetCookie
- **Response introspection**: GetStatus, IsWritten, WrittenBytes
- **Request-scoped data**: Set, Get
- **Middleware control**: Abort, IsAborted, Error, GetError

### Handler and Middleware Signatures
```go
type HandlerFunc func(*Context)
type MiddlewareFunc func(*Context, func())
type ErrorMiddlewareFunc func(error, *Context, func())
```

## Optional Modules

### WebSocket (nanite/websocket)

The websocket module provides a high-level API for WebSocket connections using gorilla/websocket.

**Basic Registration**:
```go
import "github.com/xDarkicex/nanite/websocket"

websocket.Register(r, "/ws", func(conn *websocket.Conn, c *nanite.Context) {
    defer conn.Close()
    
    // Read and echo messages
    for {
        messageType, data, err := conn.ReadMessage()
        if err != nil {
            if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
                break  // Normal close
            }
            return  // Error or abnormal close
        }
        
        // Echo the message back
        if err := conn.WriteMessage(messageType, data); err != nil {
            return
        }
    }
})
```

**Connection Methods** (from gorilla/websocket):
- `ReadMessage()` - Returns `(messageType int, data []byte, error)`. Message types: `websocket.TextMessage`, `websocket.BinaryMessage`
- `WriteMessage(messageType int, data []byte)` - Send a message
- `Close()` - Close the connection gracefully
- `SetReadDeadline(time.Time)` - Set read timeout
- `SetWriteDeadline(time.Time)` - Set write timeout
- `SetReadLimit(int64)` - Set max message size

**Close Handling**:
```go
_, data, err := conn.ReadMessage()
if err != nil {
    // Check if it's a normal close (from gorilla/websocket)
    if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
        // Client closed normally
        break
    }
    // Abnormal close or error
    return
}
```

**Configuration** (nanite/websocket APIs):
```go
// With explicit config
websocket.RegisterWithConfig(r, "/ws", handler, websocket.Config{
    ReadTimeout:    60 * time.Second,
    WriteTimeout:   10 * time.Second,
    PingInterval:   30 * time.Second,
    MaxMessageSize: 1024 * 1024,  // 1MB
    BufferSize:     4096,
    AllowedOrigins: []string{"https://example.com", "https://*.example.com"},
})

// Or with functional options
websocket.RegisterWithOptions(r, "/ws", handler,
    websocket.WithAllowedOrigins("https://example.com", "https://*.example.com"),
)
```

**Exported APIs** (with formal signatures):
```go
// Registration functions
Register(r *nanite.Router, path string, handler Handler, middleware ...nanite.MiddlewareFunc) *nanite.Router
RegisterWithConfig(r *nanite.Router, path string, handler Handler, cfg Config, middleware ...nanite.MiddlewareFunc) *nanite.Router
RegisterWithOptions(r *nanite.Router, path string, handler Handler, opts ...Option) *nanite.Router

// Note: All registration functions return *nanite.Router for method chaining
// Example: websocket.Register(r, "/ws", handler).Get("/health", healthHandler)
// 
// RegisterWithOptions does not accept middleware directly. To add middleware with functional options:
// websocket.RegisterWithConfig(r, "/ws", handler, websocket.NewConfig(opts...), middleware...)

// Configuration
type Config struct {
    ReadTimeout    time.Duration
    WriteTimeout   time.Duration
    PingInterval   time.Duration
    MaxMessageSize int64
    BufferSize     int
    AllowedOrigins []string
    Upgrader       *websocket.Upgrader
}

type Option func(*Config)
type Handler func(*websocket.Conn, *nanite.Context)

// Config builders
DefaultConfig() Config
NewConfig(opts ...Option) Config

// Functional options
WithAllowedOrigins(origins ...string) Option
WithUpgrader(upgrader *websocket.Upgrader) Option
```

**Features**:
- Automatic ping/pong keepalive (configurable interval)
- Origin validation (CORS-like checks)
- Graceful shutdown handling
- Concurrent-safe writes via `WriteControl()`
- Automatic cleanup on connection close

**Note**: `*websocket.Conn` methods like `ReadMessage()`, `WriteMessage()`, `IsCloseError()`, and message type constants come from gorilla/websocket. For advanced usage, consult the [gorilla/websocket documentation](https://pkg.go.dev/github.com/gorilla/websocket).

### HTTP/3 QUIC (nanite/quic)

The `nanite/quic` module provides HTTP/3 support using the QUIC protocol. It can run HTTP/3 only or in dual-stack mode alongside HTTP/1.1.

**Basic HTTP/3 Server**:
```go
import nanitequic "github.com/xDarkicex/nanite/quic"

r := nanite.New()
r.Get("/healthz", func(c *nanite.Context) {
    c.String(http.StatusOK, "ok")
})

// HTTP/3 only
qs := nanitequic.New(r, nanitequic.Config{
    Addr:     ":8443",
    CertFile: "server.crt",
    KeyFile:  "server.key",
})

if err := qs.StartHTTP3(); err != nil {
    panic(err)
}
```

**Dual-Stack Server (HTTP/1.1 + HTTP/3)**:
```go
// Serves HTTP/1.1 on TCP and HTTP/3 on UDP
qs := nanitequic.New(r, nanitequic.Config{
    Addr:      ":8443", // UDP port for HTTP/3
    HTTP1Addr: ":8080", // TCP port for HTTP/1.1
    CertFile:  "server.crt",
    KeyFile:   "server.key",
    HTTP1ReadHeaderTimeout: 2 * time.Second,
    HTTP1MaxHeaderBytes:    1 << 20,
})

if err := qs.StartDualAndServe(); err != nil {
    panic(err)
}
```

**Configuration Options**:
```go
cfg := nanitequic.Config{
    // HTTP/3 settings
    Addr:       ":8443",              // UDP port for HTTP/3
    CertFile:   "cert.pem",           // TLS certificate file
    KeyFile:    "key.pem",            // TLS key file
    TLSConfig:  nil,                  // Optional prebuilt TLS config
    QUICConfig: nil,                  // Optional QUIC transport tuning
    
    // HTTP/1.1 fallback settings (for dual-stack)
    HTTP1Addr:              ":8080",           // TCP port for HTTP/1.1
    HTTP1ReadTimeout:       5 * time.Second,
    HTTP1ReadHeaderTimeout: 5 * time.Second,
    HTTP1WriteTimeout:      60 * time.Second,
    HTTP1IdleTimeout:       60 * time.Second,
    HTTP1MaxHeaderBytes:    1 << 20,
    HTTP1DisableKeepAlives: false,
    
    // Logging
    Logger: func(e nanitequic.Event) {
        // e.Component: h1|h3|server
        // e.Status: started|stopped|error
        // e.Addr, e.Err, e.Time available
    },
}
```

**Alt-Svc Header Middleware**:
Advertises HTTP/3 availability to clients:
```go
r.Use(nanitequic.AltSvcNaniteMiddleware(nanitequic.AltSvcConfig{
    UDPPort: 8443,
    MaxAge:  300, // seconds
}))
```

**Helper Functions**:
```go
// Router-first helper for HTTP/3 only
err := nanitequic.StartRouterHTTP3(r, nanitequic.Config{
    Addr:     ":8443",
    CertFile: "server.crt",
    KeyFile:  "server.key",
})

// Router-first helper for dual-stack
err := nanitequic.StartRouterDual(r, nanitequic.Config{
    Addr:      ":8443",
    HTTP1Addr: ":8080",
    CertFile:  "server.crt",
    KeyFile:   "server.key",
})

// Validate TLS files before starting
if err := nanitequic.ValidateTLSFiles("server.crt", "server.key"); err != nil {
    panic(err)
}

// Load TLS config for reuse
tlsCfg, err := nanitequic.LoadTLSConfig("server.crt", "server.key", nil)
```

**Graceful Shutdown**:
```go
// With timeout
if err := qs.ShutdownGraceful(5 * time.Second); err != nil {
    log.Printf("shutdown error: %v", err)
}

// With context
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := qs.Shutdown(ctx); err != nil {
    log.Printf("shutdown error: %v", err)
}
```

**Complete Dual-Stack Example**:
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
    
    // Advertise HTTP/3 availability
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

    // Wait for interrupt signal
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig

    // Graceful shutdown
    if err := qs.ShutdownGraceful(5 * time.Second); err != nil {
        log.Printf("shutdown error: %v", err)
    }
}
```

**Production Considerations**:
- **UDP Load Balancer**: HTTP/3 requires UDP routing. Ensure your load balancer supports UDP and preserves client source addresses for QUIC.
- **Firewall Rules**: Open both TCP (for HTTP/1.1+2 fallback) and UDP (for HTTP/3), typically on the same public port.
- **Certificate Rotation**: Rotate cert/key files atomically and restart gracefully. For advanced rotation, provide `TLSConfig` with dynamic cert loading.
- **Alt-Svc Rollout**: Start with low `MaxAge` (60-300s), verify client telemetry, then increase to longer cache durations.
- **Mixed Clients**: Keep dual-stack enabled during rollout since some clients or networks block UDP/QUIC.
- **QUIC Tuning**: Use `QUICConfig` to tune idle timeouts, max streams, and buffer sizes for your workload.

**Methods**:
- `StartHTTP3()` - Start HTTP/3 only server
- `StartDualAndServe()` - Start both HTTP/1.1 and HTTP/3 servers
- `Shutdown(ctx)` - Graceful shutdown with context
- `ShutdownGraceful(timeout)` - Graceful shutdown with timeout

## Performance Notes

- Static routes use O(1) map lookup
- Dynamic routes use radix tree with LRU cache
- Context pooling via sync.Pool reduces allocations
- Fixed-size parameter arrays (default: 10 max, configurable via `WithRouteCacheOptions(size, maxParams)` or `Config.RouteMaxParams`)
- Pre-allocated validation errors
- Sharded LRU cache reduces lock contention

## Import Path

```go
import "github.com/xDarkicex/nanite"
```

For optional modules:
```go
import "github.com/xDarkicex/nanite/websocket"
import "github.com/xDarkicex/nanite/quic"
