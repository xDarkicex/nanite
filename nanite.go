// Package nanite provides a lightweight, high-performance HTTP router for Go.
// It is designed to be developer-friendly, inspired by Express.js, and optimized
// for speed and efficiency in routing, middleware handling, and WebSocket support.
package nanite

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Core Types and Data Structures

// HandlerFunc defines the signature for HTTP request handlers.
// It takes a Context pointer to process the request and send a response.
type HandlerFunc func(*Context)

// WebSocketHandler defines the signature for WebSocket handlers.
// It processes WebSocket connections using a connection and context.
type WebSocketHandler func(*websocket.Conn, *Context)

// MiddlewareFunc defines the signature for middleware functions.
// It takes a Context and a next function to control request flow.
type MiddlewareFunc func(*Context, func())

// Param represents a route parameter with a key-value pair.
// Fields are aligned for simplicity and cache efficiency.
type Param struct {
	Key   string // Parameter name
	Value string // Parameter value
}

// Context holds the state of an HTTP request and response.
// It is optimized for machine sympathy with a fixed-size array for params.
type Context struct {
	Writer         http.ResponseWriter    // Response writer for sending data
	Request        *http.Request          // Incoming HTTP request
	Params         [5]Param               // Fixed-size array for route parameters
	ParamsCount    int                    // Number of parameters used
	Values         map[string]interface{} // General-purpose value storage
	ValidationErrs ValidationErrors       // Validation errors, if any
	aborted        bool                   // Flag indicating if request is aborted
}

// ValidationError represents a single validation error with field and message.
type ValidationError struct {
	Field string `json:"field"` // Field name that failed validation
	Error string `json:"error"` // Error message describing the failure
}

// ValidationErrors is a slice of ValidationError for multiple validation failures.
type ValidationErrors []ValidationError

// Error returns a string representation of all validation errors.
func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "validation failed"
	}
	var errs []string
	for _, e := range ve {
		errs = append(errs, fmt.Sprintf("%s: %s", e.Field, e.Error))
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(errs, ", "))
}

// ValidatorFunc defines the signature for validation functions.
// It validates a string value and returns an error if invalid.
type ValidatorFunc func(string) error

// ValidationChain represents a chain of validation rules for a field.
type ValidationChain struct {
	field string          // Field name to validate
	rules []ValidatorFunc // List of validation rules
}

// Router Configuration

// Config holds configuration options for the router.
type Config struct {
	NotFoundHandler HandlerFunc           // Handler for 404 responses
	ErrorHandler    func(*Context, error) // Custom error handler
	Upgrader        *websocket.Upgrader   // WebSocket upgrader configuration
	WebSocket       *WebSocketConfig      // WebSocket-specific settings
}

// WebSocketConfig holds configuration options for WebSocket connections.
type WebSocketConfig struct {
	ReadTimeout    time.Duration // Timeout for reading messages
	WriteTimeout   time.Duration // Timeout for writing messages
	PingInterval   time.Duration // Interval for sending pings
	MaxMessageSize int64         // Maximum message size in bytes
	BufferSize     int           // Buffer size for read/write operations
}

// Router Structure

// Router is the main router type that manages HTTP and WebSocket requests.
type Router struct {
	trees      map[string]*node // Routing trees by HTTP method
	pool       sync.Pool        // Pool for reusing Context instances
	mutex      sync.RWMutex     // Mutex for thread-safe middleware updates
	middleware []MiddlewareFunc // Global middleware stack
	config     *Config          // Router configuration
	httpClient *http.Client     // HTTP client for proxying or external requests
}

// Router Initialization

// New creates a new Router instance with default configurations.
// It initializes the routing trees, context pool, and WebSocket settings.
func New() *Router {
	r := &Router{
		trees: make(map[string]*node),
		config: &Config{
			WebSocket: &WebSocketConfig{
				ReadTimeout:    60 * time.Second,
				WriteTimeout:   10 * time.Second,
				PingInterval:   30 * time.Second,
				MaxMessageSize: 1024 * 1024, // 1MB
				BufferSize:     1024,
			},
		},
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     120 * time.Second,
				DisableCompression:  false,
				ForceAttemptHTTP2:   false,
			},
			Timeout: 30 * time.Second,
		},
	}
	// Initialize context pool with pre-allocated structures
	r.pool.New = func() interface{} {
		return &Context{
			Params:  [5]Param{}, // Fixed-size array for params
			Values:  make(map[string]interface{}, 8),
			aborted: false,
		}
	}
	// Set up WebSocket upgrader with default settings
	r.config.Upgrader = &websocket.Upgrader{
		CheckOrigin:     func(*http.Request) bool { return true },
		ReadBufferSize:  r.config.WebSocket.BufferSize,
		WriteBufferSize: r.config.WebSocket.BufferSize,
	}
	return r
}

// Middleware Support

// Use adds one or more middleware functions to the router's global middleware stack.
// These middleware functions will be executed for every request in order.
func (r *Router) Use(middleware ...MiddlewareFunc) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.middleware = append(r.middleware, middleware...)
}

// Route Registration

// Get registers a handler for GET requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("GET", path, handler, middleware...)
}

// Post registers a handler for POST requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Post(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("POST", path, handler, middleware...)
}

// Put registers a handler for PUT requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Put(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("PUT", path, handler, middleware...)
}

// Delete registers a handler for DELETE requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Delete(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("DELETE", path, handler, middleware...)
}

// Patch registers a handler for PATCH requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Patch(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("PATCH", path, handler, middleware...)
}

// Options registers a handler for OPTIONS requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Options(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("OPTIONS", path, handler, middleware...)
}

// Head registers a handler for HEAD requests on the specified path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Head(path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute("HEAD", path, handler, middleware...)
}

// Handle registers a handler for the specified HTTP method and path.
// Optional middleware can be provided to preprocess the request.
func (r *Router) Handle(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.addRoute(method, path, handler, middleware...)
}

// Server Start Methods

// Start launches the HTTP server on the specified port.
// It configures timeouts and logs the server start.
func (r *Router) Start(port string) error {
	server := &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}
	fmt.Printf("Nanite server running on port %s\n", port)
	return server.ListenAndServe()
}

// StartTLS launches the HTTPS server on the specified port with TLS.
// It uses the provided certificate and key files for secure communication.
func (r *Router) StartTLS(port, certFile, keyFile string) error {
	server := &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	fmt.Printf("Nanite server running on port %s with TLS\n", port)
	return server.ListenAndServeTLS(certFile, keyFile)
}

// WebSocket Support

// WebSocket registers a WebSocket handler for the specified path.
// It upgrades the HTTP connection to WebSocket and invokes the handler.
func (r *Router) WebSocket(path string, handler WebSocketHandler, middleware ...MiddlewareFunc) {
	r.addRoute("GET", path, r.wrapWebSocketHandler(handler), middleware...)
}

// Static File Serving

// ServeStatic serves static files from the specified root directory under the given prefix.
// It supports GET and HEAD requests for file access.
func (r *Router) ServeStatic(prefix, root string) {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	fs := http.FileServer(http.Dir(root))
	handler := func(c *Context) {
		http.StripPrefix(prefix, fs).ServeHTTP(c.Writer, c.Request)
	}
	r.addRoute("GET", prefix+"/*", handler)
	r.addRoute("HEAD", prefix+"/*", handler)
}

// Validation Support

// NewValidationChain creates a new ValidationChain for the specified field.
// It is used to build a series of validation rules.
func NewValidationChain(field string) *ValidationChain {
	return &ValidationChain{field: field}
}

// Required adds a rule that the field must not be empty.
func (vc *ValidationChain) Required() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return fmt.Errorf("field is required")
		}
		return nil
	})
	return vc
}

// IsEmail adds a rule that the field must be a valid email address.
func (vc *ValidationChain) IsEmail() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil // Skip if empty unless required
		}
		if !strings.Contains(value, "@") || !strings.Contains(value, ".") {
			return fmt.Errorf("invalid email format")
		}
		return nil
	})
	return vc
}

// IsInt adds a rule that the field must be an integer.
func (vc *ValidationChain) IsInt() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("must be an integer")
		}
		return nil
	})
	return vc
}

// IsFloat adds a rule that the field must be a floating-point number.
func (vc *ValidationChain) IsFloat() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("must be a number")
		}
		return nil
	})
	return vc
}

// IsBoolean adds a rule that the field must be a boolean value.
func (vc *ValidationChain) IsBoolean() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		lowerVal := strings.ToLower(value)
		if lowerVal != "true" && lowerVal != "false" &&
			lowerVal != "1" && lowerVal != "0" {
			return fmt.Errorf("must be a boolean value")
		}
		return nil
	})
	return vc
}

// Min adds a rule that the field must be at least a specified integer value.
func (vc *ValidationChain) Min(min int) *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		num, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		if num < min {
			return fmt.Errorf("must be at least %d", min)
		}
		return nil
	})
	return vc
}

// Max adds a rule that the field must be at most a specified integer value.
func (vc *ValidationChain) Max(max int) *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		num, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("must be a number")
		}
		if num > max {
			return fmt.Errorf("must be at most %d", max)
		}
		return nil
	})
	return vc
}

// Length adds a rule that the field must have a length within the specified range.
func (vc *ValidationChain) Length(min, max int) *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		length := len(value)
		if length < min {
			return fmt.Errorf("must be at least %d characters", min)
		}
		if length > max {
			return fmt.Errorf("must be at most %d characters", max)
		}
		return nil
	})
	return vc
}

// Matches adds a rule that the field must match the specified regular expression.
func (vc *ValidationChain) Matches(pattern string) *ValidationChain {
	re, err := regexp.Compile(pattern)
	if err != nil {
		vc.rules = append(vc.rules, func(value string) error {
			return fmt.Errorf("invalid validation pattern")
		})
		return vc
	}

	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		if !re.MatchString(value) {
			return fmt.Errorf("invalid format")
		}
		return nil
	})
	return vc
}

// OneOf adds a rule that the field must be one of the specified options.
func (vc *ValidationChain) OneOf(options ...string) *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		for _, option := range options {
			if value == option {
				return nil
			}
		}
		return fmt.Errorf("must be one of: %s", strings.Join(options, ", "))
	})
	return vc
}

// Custom adds a custom validation function to the chain.
func (vc *ValidationChain) Custom(fn func(string) error) *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		return fn(value)
	})
	return vc
}

// IsArray adds a rule that the field must be a JSON array.
func (vc *ValidationChain) IsArray() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
			return fmt.Errorf("must be an array")
		}
		return nil
	})
	return vc
}

// IsObject adds a rule that the field must be a JSON object.
func (vc *ValidationChain) IsObject() *ValidationChain {
	vc.rules = append(vc.rules, func(value string) error {
		if value == "" {
			return nil
		}
		if !strings.HasPrefix(value, "{") || !strings.HasSuffix(value, "}") {
			return fmt.Errorf("must be an object")
		}
		return nil
	})
	return vc
}

// Helper Functions

// parsePath splits a path into segments for routing.
// It handles leading/trailing slashes and returns an array of path parts.
func parsePath(path string) []string {
	if path == "/" {
		return []string{}
	}

	path = strings.Trim(path, "/")
	if path == "" {
		return []string{}
	}

	parts := make([]string, 0, 10) // Most paths have < 10 segments
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	parts = append(parts, path[start:])
	return parts
}

// addRoute adds a route to the router's tree for the given method and path.
// It associates the handler and middleware with the route.
func (r *Router) addRoute(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, exists := r.trees[method]; !exists {
		r.trees[method] = &node{children: []childNode{}}
	}
	cur := r.trees[method]
	parts := parsePath(path)
	for _, part := range parts {
		var key string
		if strings.HasPrefix(part, ":") {
			key = ":"
		} else if part == "*" {
			cur.wildcard = true
			break
		} else {
			key = part
		}
		idx := sort.Search(len(cur.children), func(i int) bool { return cur.children[i].key >= key })
		if idx < len(cur.children) && cur.children[idx].key == key {
			cur = cur.children[idx].node
		} else {
			newNode := &node{path: part, children: []childNode{}}
			cur.children = insertChild(cur.children, key, newNode)
			cur = newNode
		}
	}
	// Combine global and route-specific middleware and pre-build the chain
	allMiddleware := append(r.middleware, middleware...)
	wrapped := handler
	for i := len(allMiddleware) - 1; i >= 0; i-- {
		mw := allMiddleware[i]
		next := wrapped
		wrapped = func(c *Context) {
			if !c.IsAborted() {
				mw(c, func() { next(c) })
			}
		}
	}
	cur.handler = wrapped
}

// findHandlerAndMiddleware finds the handler and parameters for a given method and path.
// It traverses the routing tree to locate the appropriate handler.
func (r *Router) findHandlerAndMiddleware(method, path string) (HandlerFunc, []Param) {
	r.mutex.RLock() // Use read lock only
	defer r.mutex.RUnlock()
	if tree, exists := r.trees[method]; exists {
		cur := tree
		var params []Param
		start := 0
		if len(path) > 0 && path[0] == '/' {
			start = 1
		}
		for i := start; i <= len(path); i++ {
			if i == len(path) || path[i] == '/' {
				segment := path[start:i]
				idx := sort.Search(len(cur.children), func(j int) bool { return cur.children[j].key >= segment })
				if idx < len(cur.children) && cur.children[idx].key == segment {
					cur = cur.children[idx].node
				} else {
					idx = sort.Search(len(cur.children), func(j int) bool { return cur.children[j].key >= ":" })
					if idx < len(cur.children) && cur.children[idx].key == ":" {
						cur = cur.children[idx].node
						params = append(params, Param{Key: cur.path[1:], Value: segment})
					} else if cur.wildcard {
						break
					} else {
						return nil, nil
					}
				}
				start = i + 1
			}
		}
		if cur.handler != nil {
			return cur.handler, params
		}
	}
	return nil, nil
}

// ServeHTTP implements the http.Handler interface for the router.
// It processes incoming HTTP requests and routes them appropriately.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Wrap the response writer to track if headers have been sent
	trackedWriter := WrapResponseWriter(w)

	// Get a context from the pool
	ctx := r.pool.Get().(*Context)
	ctx.Writer = trackedWriter
	ctx.Request = req
	ctx.ParamsCount = 0 // Reset params count
	ctx.ClearValues()
	ctx.ValidationErrs = nil
	ctx.aborted = false

	// Ensure context is returned to pool when done
	defer r.pool.Put(ctx)

	// Use the request's context for detecting cancellation and timeouts
	reqCtx := req.Context()

	// Set up a goroutine to monitor for cancellation if the context can be canceled
	if reqCtx.Done() != nil {
		// We need a way to signal when we're completely done handling this request
		finished := make(chan struct{})
		defer close(finished)

		go func() {
			select {
			case <-reqCtx.Done():
				// Request was canceled or timed out
				ctx.Abort()

				// Only send error response if we haven't already sent headers
				if !trackedWriter.Written() {
					// Determine the appropriate status code based on the error
					statusCode := http.StatusGatewayTimeout // Default to 504 for timeouts

					if reqCtx.Err() == context.Canceled {
						statusCode = 499 // Nginx's code for client closed request
					}

					http.Error(trackedWriter, fmt.Sprintf("Request %v", reqCtx.Err()), statusCode)
				}
			case <-finished:
				// Handler completed normally, do nothing
			}
		}()
	}

	// Find the appropriate handler
	handler, params := r.findHandlerAndMiddleware(req.Method, req.URL.Path)
	if handler == nil {
		if r.config.NotFoundHandler != nil {
			r.config.NotFoundHandler(ctx)
		} else {
			http.NotFound(trackedWriter, req)
		}
		return
	}

	// Set parameters to context
	for i, p := range params {
		if i < len(ctx.Params) {
			ctx.Params[i] = p
		}
	}
	ctx.ParamsCount = len(params)

	// Capture panics from handlers and provide proper error handling
	defer func() {
		if err := recover(); err != nil {
			// Abort the context to prevent further processing
			ctx.Abort()

			// Check if response was already started
			if !trackedWriter.Written() {
				if r.config.ErrorHandler != nil {
					r.config.ErrorHandler(ctx, fmt.Errorf("%v", err))
				} else {
					http.Error(trackedWriter, "Internal Server Error", http.StatusInternalServerError)
				}
			} else {
				// If headers were already sent, we can only log the error
				fmt.Printf("Panic occurred after response was started: %v\n", err)
			}
		}
	}()

	// Execute the handler (middleware chain is already pre-built in addRoute)
	handler(ctx)
}

// Context Methods

// Set stores a value in the context's value map.
func (c *Context) Set(key string, value interface{}) {
	// if c.Values == nil {
	// 	c.Values = make(map[string]interface{})
	// }
	c.Values[key] = value
}

// Get retrieves a value from the context's value map.
func (c *Context) Get(key string) interface{} {
	if c.Values != nil {
		return c.Values[key]
	}
	return nil
}

// Bind decodes the request body into the provided interface.
func (c *Context) Bind(v interface{}) error {
	if err := json.NewDecoder(c.Request.Body).Decode(v); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}
	return nil
}

// FormValue returns the value of the specified form field.
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// Query returns the value of the specified query parameter.
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// GetParam retrieves a route parameter by key.
func (c *Context) GetParam(key string) (string, bool) {
	for i := 0; i < c.ParamsCount; i++ {
		if c.Params[i].Key == key {
			return c.Params[i].Value, true
		}
	}
	return "", false
}

// MustParam retrieves a required route parameter or returns an error.
func (c *Context) MustParam(key string) (string, error) {
	if val, ok := c.GetParam(key); ok && val != "" {
		return val, nil
	}
	return "", fmt.Errorf("required parameter %s missing or empty", key)
}

// File retrieves a file from the request's multipart form.
func (c *Context) File(key string) (*multipart.FileHeader, error) {
	if c.Request.MultipartForm == nil {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, fmt.Errorf("failed to parse multipart form: %w", err)
		}
	}
	_, fh, err := c.Request.FormFile(key)
	if err != nil {
		return nil, fmt.Errorf("failed to get file %s: %w", key, err)
	}
	return fh, nil
}

// JSON sends a JSON response with the specified status code.
func (c *Context) JSON(status int, data interface{}) {
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	if err := json.NewEncoder(c.Writer).Encode(data); err != nil {
		http.Error(c.Writer, "Failed to encode JSON", http.StatusInternalServerError)
	}
}

// String sends a plain text response with the specified status code.
func (c *Context) String(status int, data string) {
	c.Writer.Header().Set("Content-Type", "text/plain")
	c.Writer.WriteHeader(status)
	c.Writer.Write([]byte(data))
}

// HTML sends an HTML response with the specified status code.
func (c *Context) HTML(status int, html string) {
	c.Writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Writer.WriteHeader(status)
	c.Writer.Write([]byte(html))
}

// SetHeader sets a header on the response writer.
func (c *Context) SetHeader(key, value string) {
	c.Writer.Header().Set(key, value)
}

// Status sets the response status code.
func (c *Context) Status(status int) {
	c.Writer.WriteHeader(status)
}

// Redirect sends a redirect response to the specified URL.
func (c *Context) Redirect(status int, url string) {
	if status < 300 || status > 399 {
		c.String(http.StatusBadRequest, "redirect status must be 3xx")
		return
	}
	c.Writer.Header().Set("Location", url)
	c.Writer.WriteHeader(status)
}

// Cookie sets a cookie on the response.
func (c *Context) Cookie(name, value string, options ...interface{}) {
	cookie := &http.Cookie{Name: name, Value: value}
	for i := 0; i < len(options)-1; i += 2 {
		if key, ok := options[i].(string); ok {
			switch key {
			case "MaxAge":
				if val, ok := options[i+1].(int); ok {
					cookie.MaxAge = val
				}
			case "Path":
				if val, ok := options[i+1].(string); ok {
					cookie.Path = val
				}
			}
		}
	}
	http.SetCookie(c.Writer, cookie)
}

// CheckValidation checks if there are validation errors and sends a response if present.
func (c *Context) CheckValidation() bool {
	if len(c.ValidationErrs) > 0 {
		c.JSON(http.StatusBadRequest, map[string]interface{}{"errors": c.ValidationErrs})
		return false
	}
	return true
}

// Abort marks the request as aborted, preventing further processing.
func (c *Context) Abort() {
	c.aborted = true
}

// IsAborted checks if the request has been aborted.
func (c *Context) IsAborted() bool {
	return c.aborted
}

// WebSocket Wrapper

// wrapWebSocketHandler wraps a WebSocketHandler into a HandlerFunc.
// It handles the WebSocket upgrade process.
func (r *Router) wrapWebSocketHandler(handler WebSocketHandler) HandlerFunc {
	return func(ctx *Context) {
		// Upgrade the connection
		conn, err := r.config.Upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			http.Error(ctx.Writer, "Failed to upgrade to WebSocket", http.StatusBadRequest)
			return
		}

		// Set connection parameters
		conn.SetReadLimit(r.config.WebSocket.MaxMessageSize)

		// Create a cancelable context for the WebSocket connection
		wsCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Create a WaitGroup to ensure all goroutines are done before returning
		var wg sync.WaitGroup

		// Create a cleanup function to handle proper connection teardown
		cleanup := func() {
			cancel()     // Cancel context to signal all goroutines to exit
			conn.Close() // Ensure connection is closed
			wg.Wait()    // Wait for all goroutines to exit
		}
		defer cleanup()

		// Set up ping/pong handlers to detect dead connections
		conn.SetPongHandler(func(string) error {
			// Reset read deadline when we get a pong response
			conn.SetReadDeadline(time.Now().Add(r.config.WebSocket.ReadTimeout))
			return nil
		})

		// Start a goroutine to periodically send pings
		wg.Add(1)
		go func() {
			defer wg.Done()
			pingTicker := time.NewTicker(r.config.WebSocket.PingInterval)
			defer pingTicker.Stop()

			for {
				select {
				case <-pingTicker.C:
					// Set write deadline for the ping
					conn.SetWriteDeadline(time.Now().Add(r.config.WebSocket.WriteTimeout))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						// Connection is probably dead, exit the goroutine
						return
					}
				case <-wsCtx.Done():
					// Context canceled, exit the goroutine
					return
				}
			}
		}()

		// Handle close signals for graceful shutdown
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Listen for server shutdown or client closure
			select {
			case <-ctx.Request.Context().Done():
				// Request context canceled (server shutting down)
				conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseGoingAway, "Server shutting down"),
					time.Now().Add(time.Second),
				)
				return
			case <-wsCtx.Done():
				// Connection context canceled (normal closure)
				return
			}
		}()

		// Set initial read deadline
		conn.SetReadDeadline(time.Now().Add(r.config.WebSocket.ReadTimeout))

		// Call the actual handler with the managed connection
		handler(conn, ctx)

		// After handler returns, ensure proper closure
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second),
		)
	}
}

// Validation Middleware

// ValidationMiddleware returns a middleware function that performs validation.
// It processes validation chains for the request and stores errors in the context.
func ValidationMiddleware(chains ...*ValidationChain) MiddlewareFunc {
	return func(ctx *Context, next func()) {
		if ctx.IsAborted() {
			return
		}

		var errs ValidationErrors

		// Process validation for all request methods that might contain data
		if len(chains) > 0 && (ctx.Request.Method == "POST" || ctx.Request.Method == "PUT" ||
			ctx.Request.Method == "PATCH" || ctx.Request.Method == "DELETE") {

			contentType := ctx.Request.Header.Get("Content-Type")

			// Handle form data
			if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
				strings.HasPrefix(contentType, "multipart/form-data") {

				if err := ctx.Request.ParseForm(); err != nil {
					errs = append(errs, ValidationError{Field: "", Error: "failed to parse form data"})
				} else {
					// Clear existing formData if it exists
					if formDataVal, exists := ctx.Values["formData"]; exists {
						if formData, ok := formDataVal.(map[string]interface{}); ok {
							// Clear the existing map instead of creating a new one
							for k := range formData {
								delete(formData, k)
							}
							// Reuse the existing map
							for key, values := range ctx.Request.Form {
								if len(values) == 1 {
									formData[key] = values[0]
								} else {
									formData[key] = values
								}
							}
						} else {
							// If formData exists but isn't the right type, replace it
							formData := make(map[string]interface{})
							for key, values := range ctx.Request.Form {
								if len(values) == 1 {
									formData[key] = values[0]
								} else {
									formData[key] = values
								}
							}
							ctx.Values["formData"] = formData
						}
					} else {
						// First time creating formData
						formData := make(map[string]interface{})
						for key, values := range ctx.Request.Form {
							if len(values) == 1 {
								formData[key] = values[0]
							} else {
								formData[key] = values
							}
						}
						ctx.Values["formData"] = formData
					}
				}
			}

			// Handle JSON content
			if strings.HasPrefix(contentType, "application/json") {
				// Get a buffer from the pool and ensure it's empty
				buffer := bufferPool.Get().(*bytes.Buffer)
				buffer.Reset()
				defer bufferPool.Put(buffer) // Return buffer to the pool when done

				// Copy request body to buffer
				if _, err := io.Copy(buffer, ctx.Request.Body); err != nil {
					errs = append(errs, ValidationError{Field: "", Error: "failed to read request body"})
				} else {
					// Create a new reader from buffer bytes and replace the request body
					bodyBytes := buffer.Bytes()
					ctx.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					// Check if "body" already exists in ctx.Values
					if bodyVal, exists := ctx.Values["body"]; exists {
						if body, ok := bodyVal.(map[string]interface{}); ok {
							// Clear the existing map instead of creating a new one
							for k := range body {
								delete(body, k)
							}
							// Parse JSON into the existing map
							if err := json.Unmarshal(bodyBytes, &body); err != nil {
								errs = append(errs, ValidationError{Field: "", Error: "invalid JSON"})
							}
						} else {
							// If it exists but isn't the right type, parse into a new map
							var body map[string]interface{}
							if err := json.Unmarshal(bodyBytes, &body); err == nil {
								ctx.Values["body"] = body
							} else {
								errs = append(errs, ValidationError{Field: "", Error: "invalid JSON"})
							}
						}
					} else {
						// First time using "body"
						var body map[string]interface{}
						if err := json.Unmarshal(bodyBytes, &body); err == nil {
							ctx.Values["body"] = body
						} else {
							errs = append(errs, ValidationError{Field: "", Error: "invalid JSON"})
						}
					}
				}
			}
		}

		// Rest of the middleware remains unchanged
		// Process validation chains
		for _, chain := range chains {
			var value interface{}
			var found bool

			// Check query parameters first
			if val := ctx.Request.URL.Query().Get(chain.field); val != "" {
				value = val
				found = true
			} else if val, ok := ctx.GetParam(chain.field); ok {
				// Then check URL parameters
				value = val
				found = true
			} else if formData, ok := ctx.Values["formData"].(map[string]interface{}); ok {
				// Check form data
				if val, ok := formData[chain.field]; ok {
					value = val
					found = true
				}
			} else if body, ok := ctx.Values["body"].(map[string]interface{}); ok {
				// Finally check JSON body fields
				if val, ok := body[chain.field]; ok {
					value = val
					found = true
				}
			}

			// Apply validation rules if the field was found
			if found {
				for _, rule := range chain.rules {
					// Convert value to string if it's not already a string
					strValue := ""
					switch v := value.(type) {
					case string:
						strValue = v
					case float64:
						strValue = strconv.FormatFloat(v, 'f', -1, 64)
					case int:
						strValue = strconv.Itoa(v)
					case bool:
						strValue = strconv.FormatBool(v)
					case []interface{}:
						// Handle array values (convert to JSON)
						if jsonBytes, err := json.Marshal(v); err == nil {
							strValue = string(jsonBytes)
						}
					case map[string]interface{}:
						// Handle object values (convert to JSON)
						if jsonBytes, err := json.Marshal(v); err == nil {
							strValue = string(jsonBytes)
						}
					default:
						// For any other type, try to convert to string
						strValue = fmt.Sprintf("%v", v)
					}

					if err := rule(strValue); err != nil {
						errs = append(errs, ValidationError{Field: chain.field, Error: err.Error()})
						break // Stop on first validation error for this field
					}
				}
			} else {
				// Field not found in any source - check if it's required
				for _, rule := range chain.rules {
					if err := rule(""); err != nil {
						errs = append(errs, ValidationError{Field: chain.field, Error: err.Error()})
						break
					}
				}
			}
		}

		// Store validation errors in context if any
		if len(errs) > 0 {
			ctx.ValidationErrs = errs
		}

		// Continue with the next middleware or handler
		next()
	}
}

// Helper Types and Functions

// childNode represents a child node in the routing tree.
type childNode struct {
	key  string
	node *node
}

// node represents a node in the routing tree.
type node struct {
	path       string
	wildcard   bool
	handler    HandlerFunc
	children   []childNode
	middleware []MiddlewareFunc
}

// insertChild inserts a child node into the sorted list of children.
func insertChild(children []childNode, key string, node *node) []childNode {
	idx := sort.Search(len(children), func(i int) bool { return children[i].key >= key })
	if idx < len(children) && children[idx].key == key {
		children[idx].node = node
	} else {
		newChild := childNode{key: key, node: node}
		children = append(children, childNode{})
		copy(children[idx+1:], children[idx:])
		children[idx] = newChild
	}
	return children
}

// TrackedResponseWriter wraps http.ResponseWriter to track if headers have been sent.
type TrackedResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
	bytesWritten  int64
}

// WrapResponseWriter creates a new TrackedResponseWriter.
func WrapResponseWriter(w http.ResponseWriter) *TrackedResponseWriter {
	return &TrackedResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

// WriteHeader records that headers have been written.
func (w *TrackedResponseWriter) WriteHeader(statusCode int) {
	if !w.headerWritten {
		w.statusCode = statusCode
		w.ResponseWriter.WriteHeader(statusCode)
		w.headerWritten = true
	}
}

// Write records that data (and implicitly headers) have been written.
func (w *TrackedResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

// Status returns the HTTP status code that was set.
func (w *TrackedResponseWriter) Status() int {
	return w.statusCode
}

// Written returns whether headers have been sent.
func (w *TrackedResponseWriter) Written() bool {
	return w.headerWritten
}

// BytesWritten returns the number of bytes written.
func (w *TrackedResponseWriter) BytesWritten() int64 {
	return w.bytesWritten
}

// Unwrap returns the original ResponseWriter.
func (w *TrackedResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush implements http.Flusher interface if the underlying writer supports it.
func (w *TrackedResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements http.Hijacker interface if the underlying writer supports it.
func (w *TrackedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// Push implements http.Pusher interface if the underlying writer supports it.
func (w *TrackedResponseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return fmt.Errorf("underlying ResponseWriter does not implement http.Pusher")
}

// Buffer Pool for ValidationMiddleware
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// ClearValues efficiently clears the Values map without reallocating
func (c *Context) ClearValues() {
	for k := range c.Values {
		delete(c.Values, k)
	}
}
