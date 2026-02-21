// Package nanite provides a high-performance HTTP router with Express-like ergonomics.
package nanite

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"

	"strings"
	"sync"
	"time"
)

type ErrorMiddlewareFunc func(err error, ctx *Context, next func())

type HandlerFunc func(*Context)

type MiddlewareFunc func(*Context, func())

type Param struct {
	Key   string // Parameter name as defined in route pattern (e.g., ":id")
	Value string // Value extracted from request path (raw string)
}

type Context struct {
	// Core HTTP objects
	Writer  http.ResponseWriter // Underlying response writer for sending HTTP responses
	Request *http.Request       // Original HTTP request with all headers, body, and URL information

	// Reference maps (8-byte pointers)
	Values     map[string]interface{} // Thread-safe key-value store for request-scoped data sharing between handlers and middleware
	lazyFields map[string]*LazyField  // Deferred validation fields that only evaluate when accessed, reducing unnecessary processing

	// Array and slice fields
	Params         [10]Param        // Fixed-size array of route parameters extracted from URL (e.g., /users/:id → {id: "123"})
	ValidationErrs ValidationErrors // Collection of validation failures for providing consistent error responses

	// Integer fields (8 bytes on 64-bit systems)
	ParamsCount int // Number of active parameters in the Params array, avoids unnecessary iterations

	// Boolean flags (1 byte + potential padding)
	aborted    bool  // Request termination flag that stops middleware chain execution when true
	requestErr error // Hot-path error storage to avoid map lookups in ServeHTTP

	// Middleware execution state (reused per request to avoid per-call closure allocs)
	middlewareChain []MiddlewareFunc
	finalHandler    HandlerFunc
	middlewareIndex int
	nextMiddleware  func()

	// Reusable request-body wrapper for middleware that needs to preserve body reads.
	reusedBody reusableRequestBody

	// Tracked pooled maps to avoid iterating Values on every request cleanup.
	pooledMapKeys   [4]string
	pooledMapValues [4]map[string]interface{}
	pooledMapCount  int
}

type reusableRequestBody struct {
	reader bytes.Reader
}

func (r *reusableRequestBody) Read(p []byte) (int, error) {
	return r.reader.Read(p)
}

func (r *reusableRequestBody) Close() error {
	return nil
}

func (r *reusableRequestBody) Reset(b []byte) {
	r.reader.Reset(b)
}

type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "validation failed"
	}
	var errs []string
	for _, e := range ve {
		errs = append(errs, fmt.Sprintf("%s: %s", e.Field, e.Error()))
	}
	return fmt.Sprintf("validation failed: %s", strings.Join(errs, ", "))
}

// ### Router Configuration

type Config struct {
	NotFoundHandler      HandlerFunc
	ErrorHandler         func(*Context, error)
	RecoverPanics        bool
	RouteCacheSize       int
	RouteMaxParams       int
	RouteCachePromote    uint32
	RouteCacheMinDyn     int
	DefaultBufferSize    int
	TextBufferSize       int
	BinaryBufferSize     int
	AdaptiveBuffering    bool
	ServerReadTimeout    time.Duration
	ServerWriteTimeout   time.Duration
	ServerIdleTimeout    time.Duration
	ServerMaxHeaderBytes int
	TCPReadBuffer        int
	TCPWriteBuffer       int
}

type staticRoute struct {
	handler HandlerFunc
	params  []Param
}

const radixChildSlots = 256
const paramVariantSlots = 4

// RadixNode is an adaptive radix tree node optimized for HTTP routing.
// Static children are addressed directly by first byte for O(1) lookup.
type RadixNode struct {
	prefix        string
	handler       HandlerFunc
	children      [radixChildSlots]*RadixNode
	childCount    int
	paramChildren [paramVariantSlots]*RadixNode
	paramCount    int
	wildcardChild *RadixNode
	paramName     string
	paramSuffix   string
	wildcardName  string
	maxDepth      int // Track subtree depth for rebalancing
}

type RouterGroup struct {
	prefix     string
	parent     *Router
	middleware []MiddlewareFunc
}

func (g *RouterGroup) Use(middleware ...MiddlewareFunc) *RouterGroup {
	g.middleware = append(g.middleware, middleware...)
	return g
}

func (g *RouterGroup) Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *RouterGroup {
	fullPath := g.prefix + path
	g.parent.addRoute("GET", fullPath, handler, append(g.middleware, middleware...)...)
	return g
}

func (g *RouterGroup) Post(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *RouterGroup {
	fullPath := g.prefix + path
	g.parent.addRoute("POST", fullPath, handler, append(g.middleware, middleware...)...)
	return g
}

func (g *RouterGroup) Put(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *RouterGroup {
	fullPath := g.prefix + path
	g.parent.addRoute("PUT", fullPath, handler, append(g.middleware, middleware...)...)
	return g
}

func (g *RouterGroup) Delete(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *RouterGroup {
	fullPath := g.prefix + path
	g.parent.addRoute("DELETE", fullPath, handler, append(g.middleware, middleware...)...)
	return g
}

type Router struct {
	staticRoutes               map[string]map[string]staticRoute
	trees                      map[string]*RadixNode
	dynamicCounts              map[string]int
	namedRoutes                map[string]namedRoute
	routeCache                 *LRUCache
	Pool                       sync.Pool // Exported for testing
	middleware                 []MiddlewareFunc
	errorMiddleware            []ErrorMiddlewareFunc
	config                     *Config
	httpClient                 *http.Client
	server                     *http.Server
	shutdownHooks              []ShutdownHook
	mutex                      sync.RWMutex
	groups                     map[string]*RouterGroup
	routeCacheMinDynamicRoutes int
}
type ShutdownHook func() error

func New(opts ...Option) *Router {
	r := &Router{
		trees:         make(map[string]*RadixNode),
		staticRoutes:  make(map[string]map[string]staticRoute),
		dynamicCounts: make(map[string]int),
		namedRoutes:   make(map[string]namedRoute),
		groups:        make(map[string]*RouterGroup),
		config: &Config{
			RecoverPanics:        true,
			RouteCacheSize:       1024, // Default cache size
			RouteMaxParams:       10,   // Default max params
			RouteCachePromote:    4,
			RouteCacheMinDyn:     8,
			ServerReadTimeout:    5 * time.Second,
			ServerWriteTimeout:   60 * time.Second,
			ServerIdleTimeout:    60 * time.Second,
			ServerMaxHeaderBytes: 1 << 20,
			TCPReadBuffer:        65536,
			TCPWriteBuffer:       65536,
		},
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        1000,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     120 * time.Second,
				DisableCompression:  false,
				ForceAttemptHTTP2:   false,
				DialContext: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
			Timeout: 30 * time.Second,
		},
		routeCacheMinDynamicRoutes: 8,
	}

	// Context pool with memory-efficient initialization
	r.Pool.New = func() interface{} {
		ctx := &Context{
			Params:     [10]Param{},                     // Pre-allocated parameter array
			Values:     make(map[string]interface{}, 8), // Common case: 8-12 context values
			lazyFields: make(map[string]*LazyField),     // Lazy response formatting
			Writer:     &responseWriter{},               // Initialize with empty writer
			aborted:    false,
		}

		ctx.nextMiddleware = func() {
			if ctx.aborted {
				return
			}

			if ctx.middlewareIndex < len(ctx.middlewareChain) {
				current := ctx.middlewareChain[ctx.middlewareIndex]
				ctx.middlewareIndex++
				current(ctx, ctx.nextMiddleware)
				return
			}

			if ctx.finalHandler != nil {
				handler := ctx.finalHandler
				ctx.finalHandler = nil
				handler(ctx)
			}
		}

		return ctx
	}

	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}

	r.ensureConfigDefaults()

	return r
}

func (r *Router) ensureConfigDefaults() {
	if r.config == nil {
		r.config = &Config{}
	}

	if r.config.RouteCacheSize == 0 {
		r.config.RouteCacheSize = 1024
	}
	if r.config.RouteMaxParams == 0 {
		r.config.RouteMaxParams = 10
	}
	if r.config.RouteCachePromote == 0 {
		r.config.RouteCachePromote = 4
	}
	if r.config.RouteCacheMinDyn < 0 {
		r.config.RouteCacheMinDyn = 0
	}
	if r.config.RouteCacheMinDyn == 0 {
		r.config.RouteCacheMinDyn = 8
	}
	if r.config.ServerReadTimeout == 0 {
		r.config.ServerReadTimeout = 5 * time.Second
	}
	if r.config.ServerWriteTimeout == 0 {
		r.config.ServerWriteTimeout = 60 * time.Second
	}
	if r.config.ServerIdleTimeout == 0 {
		r.config.ServerIdleTimeout = 60 * time.Second
	}
	if r.config.ServerMaxHeaderBytes == 0 {
		r.config.ServerMaxHeaderBytes = 1 << 20
	}
	if r.config.TCPReadBuffer == 0 {
		r.config.TCPReadBuffer = 65536
	}
	if r.config.TCPWriteBuffer == 0 {
		r.config.TCPWriteBuffer = 65536
	}

	// Route cache optimized for high locality workloads
	if r.config.RouteCacheSize <= 0 {
		r.routeCache = nil
	} else {
		r.routeCache = NewLRUCache(r.config.RouteCacheSize, r.config.RouteMaxParams)
		r.routeCache.SetPromoteEvery(r.config.RouteCachePromote)
	}
	r.routeCacheMinDynamicRoutes = r.config.RouteCacheMinDyn
}

func (r *Router) Use(middleware ...MiddlewareFunc) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.middleware = append(r.middleware, middleware...)
}

func (r *Router) Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("GET", path, handler, middleware...)
	return r
}

func (r *Router) Post(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("POST", path, handler, middleware...)
	return r
}

func (r *Router) Put(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("PUT", path, handler, middleware...)
	return r
}

func (r *Router) Delete(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("DELETE", path, handler, middleware...)
	return r
}

func (r *Router) Patch(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("PATCH", path, handler, middleware...)
	return r
}

func (r *Router) Options(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("OPTIONS", path, handler, middleware...)
	return r
}

func (r *Router) Head(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute("HEAD", path, handler, middleware...)
	return r
}

func (r *Router) Handle(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	r.addRoute(method, path, handler, middleware...)
	return r
}

func (r *Router) Start(port string) error {
	r.mutex.Lock()
	if r.server != nil {
		r.mutex.Unlock()
		return fmt.Errorf("server already running")
	}
	r.ensureConfigDefaults()
	r.server = &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    r.config.ServerReadTimeout,
		WriteTimeout:   r.config.ServerWriteTimeout,
		IdleTimeout:    r.config.ServerIdleTimeout,
		MaxHeaderBytes: r.config.ServerMaxHeaderBytes,
		ConnState: func(conn net.Conn, state http.ConnState) {
			if state == http.StateNew {
				tcpConn, ok := conn.(*net.TCPConn)
				if ok {
					if r.config.TCPReadBuffer > 0 {
						_ = tcpConn.SetReadBuffer(r.config.TCPReadBuffer)
					}
					if r.config.TCPWriteBuffer > 0 {
						_ = tcpConn.SetWriteBuffer(r.config.TCPWriteBuffer)
					}
				}
			}
		},
	}
	r.mutex.Unlock()
	fmt.Printf("Nanite server running on port %s\n", port)
	err := r.server.ListenAndServe()
	// ErrServerClosed is returned when Shutdown is called
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (r *Router) AddShutdownHook(hook ShutdownHook) *Router {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.shutdownHooks = append(r.shutdownHooks, hook)
	return r
}

func (r *Router) StartTLS(port, certFile, keyFile string) error {
	r.mutex.Lock()
	if r.server != nil {
		r.mutex.Unlock()
		return fmt.Errorf("server already running")
	}
	r.ensureConfigDefaults()

	r.server = &http.Server{
		Addr:           ":" + port,
		Handler:        r,
		ReadTimeout:    r.config.ServerReadTimeout,
		WriteTimeout:   r.config.ServerWriteTimeout,
		IdleTimeout:    r.config.ServerIdleTimeout,
		MaxHeaderBytes: r.config.ServerMaxHeaderBytes,
	}
	r.mutex.Unlock()
	err := r.server.ListenAndServeTLS(certFile, keyFile)
	// ErrServerClosed is returned when Shutdown is called
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (r *Router) Shutdown(timeout time.Duration) error {
	r.mutex.Lock()
	if r.server == nil {
		r.mutex.Unlock()
		return fmt.Errorf("server not started or already shut down")
	}

	// Execute shutdown hooks
	for _, hook := range r.shutdownHooks {
		if err := hook(); err != nil {
			fmt.Printf("Error during shutdown hook: %v\n", err)
		}
	}

	server := r.server
	r.server = nil
	r.mutex.Unlock()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Perform graceful shutdown
	return server.Shutdown(ctx)
}

func (r *Router) ShutdownImmediate() error {
	r.mutex.Lock()
	if r.server == nil {
		r.mutex.Unlock()
		return fmt.Errorf("server not started or already shut down")
	}

	server := r.server
	r.server = nil
	r.mutex.Unlock()

	// Close immediately
	return server.Close()
}

func (r *Router) ServeStatic(prefix, root string) *Router {
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	fs := http.FileServer(http.Dir(root))
	handler := func(c *Context) {
		http.StripPrefix(prefix, fs).ServeHTTP(c.Writer, c.Request)
	}
	r.addRoute("GET", prefix+"/*", handler)
	r.addRoute("HEAD", prefix+"/*", handler)
	return r
}

func (r *Router) addRoute(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Combine global and route middleware
	routeMiddleware := make([]MiddlewareFunc, len(r.middleware)+len(middleware))
	copy(routeMiddleware, r.middleware)
	copy(routeMiddleware[len(r.middleware):], middleware)

	wrapped := handler
	if len(routeMiddleware) > 0 {
		chain := routeMiddleware
		finalHandler := handler
		wrapped = func(c *Context) {
			c.runMiddlewareChain(finalHandler, chain)
		}
	}

	// Check if route is static (no parameters or wildcards)
	isStatic := !strings.Contains(path, ":") && !strings.Contains(path, "*")

	// Store static routes in map for O(1) lookup
	if isStatic {
		if _, exists := r.staticRoutes[method]; !exists {
			r.staticRoutes[method] = make(map[string]staticRoute)
		}

		r.staticRoutes[method][path] = staticRoute{handler: wrapped, params: []Param{}}
		return // Skip radix tree insertion for static routes
	}

	// Only dynamic routes go in the radix tree
	if _, exists := r.trees[method]; !exists {
		r.trees[method] = &RadixNode{
			prefix: "",
		}
	}

	// Insert into radix tree
	root := r.trees[method]
	r.dynamicCounts[method]++
	if path == "" || path == "/" {
		root.handler = wrapped
	} else {
		if path[0] != '/' {
			path = "/" + path
		}

		root.insertRoute(path[1:], wrapped) // Skip leading slash
	}
}

func (r *Router) findHandlerAndMiddlewareWithParams(method, path string, params []Param) (HandlerFunc, []Param) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Fast path: check static routes first (O(1) lookup)
	if methodRoutes, exists := r.staticRoutes[method]; exists {
		if route, found := methodRoutes[path]; found {
			return route.handler, route.params
		}
	}

	// Second tier: check LRU cache before doing the more expensive radix tree lookup
	// This avoids the cost of tree traversal for frequently used dynamic routes
	useRouteCache := r.routeCache != nil && r.dynamicCounts[method] >= r.routeCacheMinDynamicRoutes
	if useRouteCache {
		if handler, params, found := r.routeCache.Get(method, path); found {
			// Cache hit - strings are interned and params are pooled for efficiency
			return handler, params
		}
		// Cache miss is tracked internally by the LRU implementation
	}

	// Third tier: use radix tree for dynamic routes
	if tree, exists := r.trees[method]; exists {
		params = params[:0]

		searchPath := path
		if len(path) > 0 && path[0] == '/' {
			searchPath = path[1:]
		}

		handler, params := tree.findRoute(searchPath, params)

		// Cache successful lookups to speed up future requests
		// The LRU handles memory management, parameter pooling, and string interning
		if handler != nil && useRouteCache {
			cachedParams := getParamSlice(len(params))
			cachedParams = append(cachedParams, params...)
			r.routeCache.Add(method, path, handler, cachedParams)
		}

		return handler, params
	}

	// No matching route found
	return nil, nil
}

func (r *Router) FindHandlerAndMiddleware(method, path string) (HandlerFunc, []Param) { // Exported for testing
	return r.findHandlerAndMiddlewareWithParams(method, path, make([]Param, 0, 5))
}

// Pools for zero-allocation request handling
var responseWriterPool = sync.Pool{
	New: func() interface{} {
		return &TrackedResponseWriter{}
	},
}

var bufferedWriterPool = sync.Pool{
	New: func() interface{} {
		return &BufferedResponseWriter{
			buffer: new(bytes.Buffer),
		}
	},
}

var defaultNotFoundBody = []byte("404 page not found\n")

func writeDefaultNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write(defaultNotFoundBody)
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if r.config != nil && !r.config.RecoverPanics {
		r.serveHTTPNoRecover(w, req)
		return
	}

	// Get pooled response writers
	trackedWriter := responseWriterPool.Get().(*TrackedResponseWriter)
	trackedWriter.ResponseWriter = w
	trackedWriter.statusCode = http.StatusOK
	trackedWriter.headerWritten = false
	trackedWriter.bytesWritten = 0

	bufferedWriter := bufferedWriterPool.Get().(*BufferedResponseWriter)
	bufferedWriter.TrackedResponseWriter = trackedWriter
	bufferedWriter.autoFlush = true
	bufferedWriter.buffer.Reset()
	bufferedWriter.bufferSize = DefaultBufferSize

	// Get a context from the pool and initialize it with a single Reset call
	ctx := r.Pool.Get().(*Context)
	ctx.Reset(bufferedWriter, req)

	// Use a single defer for panic recovery and pooled resource cleanup.
	defer func() {
		if err := recover(); err != nil {
			ctx.Abort()
			if !trackedWriter.Written() {
				// Convert panic value to error
				var errValue error
				switch e := err.(type) {
				case error:
					errValue = e
				default:
					errValue = fmt.Errorf("%v", err)
				}

				r.mutex.RLock()
				hasErrorMiddleware := len(r.errorMiddleware) > 0
				r.mutex.RUnlock()

				if hasErrorMiddleware {
					r.mutex.RLock()
					executeErrorMiddlewareChain(errValue, ctx, r.errorMiddleware)
					r.mutex.RUnlock()
				} else if r.config.ErrorHandler != nil {
					r.config.ErrorHandler(ctx, errValue)
				} else {
					http.Error(trackedWriter, "Internal Server Error", http.StatusInternalServerError)
				}
			} else {
				fmt.Printf("Panic occurred after response started: %v\n", err)
			}
			bufferedWriter.Close()
		}
		ctx.CleanupPooledResources()
		r.Pool.Put(ctx)
		bufferedWriterPool.Put(bufferedWriter)
		responseWriterPool.Put(trackedWriter)
	}()

	// Route lookup: find the appropriate handler and parameters
	var paramsBuf [10]Param
	handler, params := r.findHandlerAndMiddlewareWithParams(req.Method, req.URL.Path, paramsBuf[:0])

	// Copy params to context's fixed-size array (no allocation)
	for i, p := range params {
		if i < len(ctx.Params) {
			ctx.Params[i] = p
		}
	}
	ctx.ParamsCount = len(params)

	if handler == nil {
		// Handle 404 Not Found
		if r.config.NotFoundHandler != nil {
			r.config.NotFoundHandler(ctx)
		} else {
			writeDefaultNotFound(trackedWriter)
		}
		bufferedWriter.Close()
		return
	}

	// Execute the wrapped handler (already includes all middleware)
	handler(ctx)

	// Check if the context contains an error to be handled by error middleware
	if err := ctx.GetError(); err != nil && !trackedWriter.Written() {
		r.mutex.RLock()
		hasErrorMiddleware := len(r.errorMiddleware) > 0
		r.mutex.RUnlock()

		if hasErrorMiddleware {
			r.mutex.RLock()
			executeErrorMiddlewareChain(err, ctx, r.errorMiddleware)
			r.mutex.RUnlock()
		} else if r.config.ErrorHandler != nil {
			r.config.ErrorHandler(ctx, err)
		}
	} else if ctx.IsAborted() && !trackedWriter.Written() {
		// Handle aborted requests that haven't written a response
		if r.config.NotFoundHandler != nil {
			r.config.NotFoundHandler(ctx)
		} else {
			writeDefaultNotFound(trackedWriter)
		}
	}

	// Ensure the buffered writer is closed and flushed
	bufferedWriter.Close()
}

func (r *Router) serveHTTPNoRecover(w http.ResponseWriter, req *http.Request) {
	trackedWriter := responseWriterPool.Get().(*TrackedResponseWriter)
	trackedWriter.ResponseWriter = w
	trackedWriter.statusCode = http.StatusOK
	trackedWriter.headerWritten = false
	trackedWriter.bytesWritten = 0

	bufferedWriter := bufferedWriterPool.Get().(*BufferedResponseWriter)
	bufferedWriter.TrackedResponseWriter = trackedWriter
	bufferedWriter.autoFlush = true
	bufferedWriter.buffer.Reset()
	bufferedWriter.bufferSize = DefaultBufferSize

	ctx := r.Pool.Get().(*Context)
	ctx.Reset(bufferedWriter, req)

	cleanup := func() {
		ctx.CleanupPooledResources()
		r.Pool.Put(ctx)
		bufferedWriterPool.Put(bufferedWriter)
		responseWriterPool.Put(trackedWriter)
	}

	var paramsBuf [10]Param
	handler, params := r.findHandlerAndMiddlewareWithParams(req.Method, req.URL.Path, paramsBuf[:0])

	for i, p := range params {
		if i < len(ctx.Params) {
			ctx.Params[i] = p
		}
	}
	ctx.ParamsCount = len(params)

	if handler == nil {
		if r.config.NotFoundHandler != nil {
			r.config.NotFoundHandler(ctx)
		} else {
			writeDefaultNotFound(trackedWriter)
		}
		bufferedWriter.Close()
		cleanup()
		return
	}

	handler(ctx)

	if err := ctx.GetError(); err != nil && !trackedWriter.Written() {
		r.mutex.RLock()
		hasErrorMiddleware := len(r.errorMiddleware) > 0
		r.mutex.RUnlock()

		if hasErrorMiddleware {
			r.mutex.RLock()
			executeErrorMiddlewareChain(err, ctx, r.errorMiddleware)
			r.mutex.RUnlock()
		} else if r.config.ErrorHandler != nil {
			r.config.ErrorHandler(ctx, err)
		}
	} else if ctx.IsAborted() && !trackedWriter.Written() {
		if r.config.NotFoundHandler != nil {
			r.config.NotFoundHandler(ctx)
		} else {
			writeDefaultNotFound(trackedWriter)
		}
	}

	bufferedWriter.Close()
	cleanup()
}

//go:inline
func longestCommonPrefix(a, b string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}

	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			return i
		}
	}

	return max
}

func (n *RadixNode) findChild(key byte) *RadixNode {
	return n.children[key]
}

// addChild adds a child node with the given key.
func (n *RadixNode) addChild(key byte, child *RadixNode) {
	if n.children[key] == nil {
		n.childCount++
	}
	n.children[key] = child
}

func (n *RadixNode) hasParamVariants() bool {
	return n.paramCount > 0
}

func (n *RadixNode) findParamVariant(paramName, paramSuffix string) *RadixNode {
	for i := 0; i < n.paramCount; i++ {
		child := n.paramChildren[i]
		if child != nil && child.paramName == paramName && child.paramSuffix == paramSuffix {
			return child
		}
	}
	return nil
}

// findRoute searches for a route in the radix tree.
func (n *RadixNode) findRoute(path string, params []Param) (HandlerFunc, []Param) {
	// Base case: empty path
	if path == "" {
		return n.handler, params
	}

	// Try static children first.
	if len(path) > 0 {
		firstByte := path[0]
		child := n.findChild(firstByte)
		if child != nil && len(child.prefix) > 0 {
			pl := len(child.prefix)
			if len(path) >= pl && path[:pl] == child.prefix {
				subPath := path[pl:]
				if len(subPath) > 0 && subPath[0] == '/' {
					subPath = subPath[1:]
				}
				if handler, subParams := child.findRoute(subPath, params); handler != nil {
					return handler, subParams
				}
			}
		}
	}

	// Try parameter variants.
	hasParamVariants := n.hasParamVariants()
	if hasParamVariants {
		i := 0
		for i < len(path) && path[i] != '/' {
			i++
		}
		if i == 0 {
			return nil, nil
		}

		paramSegment := path[:i]
		hasRemaining := i < len(path)
		var remainingPath string
		if hasRemaining {
			remainingPath = path[i+1:]
		}

		// Exact suffix matches first; longer suffixes win to avoid ambiguous overlap.
		prevLen := len(paramSegment) + 1
		for {
			bestIdx := -1
			bestLen := -1
			for pi := 0; pi < n.paramCount; pi++ {
				paramNode := n.paramChildren[pi]
				if paramNode == nil || paramNode.paramSuffix == "" {
					continue
				}
				suffixLen := len(paramNode.paramSuffix)
				if suffixLen <= 0 || suffixLen >= len(paramSegment) || suffixLen >= prevLen {
					continue
				}
				if strings.HasSuffix(paramSegment, paramNode.paramSuffix) && suffixLen > bestLen {
					bestLen = suffixLen
					bestIdx = pi
				}
			}
			if bestIdx == -1 {
				break
			}

			paramNode := n.paramChildren[bestIdx]
			paramValue := paramSegment[:len(paramSegment)-len(paramNode.paramSuffix)]
			if paramValue != "" {
				newParams := append(params, Param{Key: paramNode.paramName, Value: paramValue})
				if !hasRemaining {
					if paramNode.handler != nil {
						return paramNode.handler, newParams
					}
				} else if handler, subParams := paramNode.findRoute(remainingPath, newParams); handler != nil {
					return handler, subParams
				}
			}
			prevLen = bestLen
		}

		// Fallback to plain parameter variant (no suffix) if present.
		for pi := 0; pi < n.paramCount; pi++ {
			paramNode := n.paramChildren[pi]
			if paramNode == nil || paramNode.paramSuffix != "" {
				continue
			}
			newParams := append(params, Param{Key: paramNode.paramName, Value: paramSegment})
			if !hasRemaining {
				if paramNode.handler != nil {
					return paramNode.handler, newParams
				}
			} else if handler, subParams := paramNode.findRoute(remainingPath, newParams); handler != nil {
				return handler, subParams
			}
			break
		}
	}

	// Wildcard only when no parameter variants exist.
	if n.wildcardChild != nil && !hasParamVariants {
		newParams := append(params, Param{Key: n.wildcardChild.wildcardName, Value: path})
		return n.wildcardChild.handler, newParams
	}

	return nil, nil
}

// insertRoute inserts a route into the radix tree.
func (n *RadixNode) insertRoute(path string, handler HandlerFunc) {
	// Base case: empty path
	if path == "" {
		n.handler = handler
		n.updateDepth()
		return
	}

	// Handle parameters (:id)
	if path[0] == ':' {
		// Extract parameter name and remaining path
		paramEnd := strings.IndexByte(path, '/')
		var paramName, paramSuffix, remainingPath string

		if paramEnd == -1 {
			segment := path[1:]
			if suffixStart := strings.IndexByte(segment, ':'); suffixStart != -1 {
				paramName = segment[:suffixStart]
				paramSuffix = segment[suffixStart:]
			} else {
				paramName = segment
			}
			remainingPath = ""
		} else {
			segment := path[1:paramEnd]
			if suffixStart := strings.IndexByte(segment, ':'); suffixStart != -1 {
				paramName = segment[:suffixStart]
				paramSuffix = segment[suffixStart:]
			} else {
				paramName = segment
			}
			remainingPath = path[paramEnd:]
		}
		if paramName == "" {
			return
		}

		paramNode := n.findParamVariant(paramName, paramSuffix)
		if paramNode == nil {
			// Keep suffix unique at each depth to avoid ambiguous captures with different key names.
			for i := 0; i < n.paramCount; i++ {
				existing := n.paramChildren[i]
				if existing != nil && existing.paramSuffix == paramSuffix && existing.paramName != paramName {
					return
				}
			}
			if n.paramCount >= paramVariantSlots {
				return
			}
			paramNode = &RadixNode{
				prefix:      ":" + paramName + paramSuffix,
				paramName:   paramName,
				paramSuffix: paramSuffix,
			}
			n.paramChildren[n.paramCount] = paramNode
			n.paramCount++
		}

		if paramNode.paramName != paramName || paramNode.paramSuffix != paramSuffix {
			return
		}

		// Continue with remaining path
		if remainingPath == "" {
			paramNode.handler = handler
		} else {
			paramNode.insertRoute(remainingPath, handler)
		}

		n.updateDepth()
		return
	}

	// Handle wildcards (*path)
	if path[0] == '*' {
		n.wildcardChild = &RadixNode{
			prefix:       path,
			handler:      handler,
			wildcardName: path[1:],
		}
		n.updateDepth()
		return
	}

	// Find the first differing character
	var i int
	for i = 0; i < len(path); i++ {
		if path[i] == '/' || path[i] == ':' || path[i] == '*' {
			break
		}
	}

	// Extract the current segment
	segment := path[:i]
	remainingPath := ""
	if i < len(path) {
		remainingPath = path[i:]
	}

	// Add check for empty segment to prevent index out of range panic
	if len(segment) == 0 {
		// Skip empty segments and continue with remaining path
		if remainingPath != "" && len(remainingPath) > 0 {
			// If remainingPath starts with a slash, skip it
			if remainingPath[0] == '/' {
				remainingPath = remainingPath[1:]
			}
			n.insertRoute(remainingPath, handler)
			return
		}
		// If no remaining path, set handler on current node
		n.handler = handler
		n.updateDepth()
		return
	}

	// Look for matching child.
	firstByte := segment[0]
	c := n.findChild(firstByte)
	if c == nil {
		// Create new child
		c = &RadixNode{
			prefix: segment,
		}
		n.addChild(firstByte, c)

		// Set handler or continue with remaining path
		if remainingPath == "" {
			c.handler = handler
		} else {
			c.insertRoute(remainingPath, handler)
		}
		n.updateDepth()
		return
	}

	// Find common prefix length
	commonPrefixLen := longestCommonPrefix(c.prefix, segment)

	if commonPrefixLen == len(c.prefix) {
		// Child prefix is completely contained in this segment
		if commonPrefixLen == len(segment) {
			// Exact match, continue with remaining path
			if remainingPath == "" {
				c.handler = handler
			} else {
				c.insertRoute(remainingPath, handler)
			}
		} else {
			// Current segment extends beyond child prefix
			c.insertRoute(segment[commonPrefixLen:]+remainingPath, handler)
		}
	} else {
		// Need to split the node
		child := &RadixNode{
			prefix:        c.prefix[commonPrefixLen:],
			handler:       c.handler,
			children:      c.children,
			childCount:    c.childCount,
			paramChildren: c.paramChildren,
			paramCount:    c.paramCount,
			wildcardChild: c.wildcardChild,
			paramName:     c.paramName,
			paramSuffix:   c.paramSuffix,
			wildcardName:  c.wildcardName,
			maxDepth:      c.maxDepth,
		}

		// Reset the original child
		c.prefix = c.prefix[:commonPrefixLen]
		c.handler = nil
		c.children = [radixChildSlots]*RadixNode{}
		c.childCount = 0
		c.paramChildren = [paramVariantSlots]*RadixNode{}
		c.paramCount = 0
		c.wildcardChild = nil
		c.paramName = ""
		c.paramSuffix = ""
		c.wildcardName = ""
		c.maxDepth = 0

		// Add the split node as a child
		childFirstByte := child.prefix[0]
		c.addChild(childFirstByte, child)

		// Handle current path
		if commonPrefixLen == len(segment) {
			// Current segment matches prefix exactly
			if remainingPath == "" {
				c.handler = handler
			} else {
				c.insertRoute(remainingPath, handler)
			}
		} else {
			// Current segment extends beyond common prefix
			newChild := &RadixNode{
				prefix: segment[commonPrefixLen:],
			}

			if remainingPath == "" {
				newChild.handler = handler
			} else {
				newChild.insertRoute(remainingPath, handler)
			}

			newFirstByte := newChild.prefix[0]
			c.addChild(newFirstByte, newChild)
		}
	}

	n.updateDepth()
}

// updateDepth recalculates the max depth of this node's subtree
func (n *RadixNode) updateDepth() {
	maxChildDepth := 0

	for i := 0; i < radixChildSlots; i++ {
		child := n.children[i]
		if child != nil && child.maxDepth > maxChildDepth {
			maxChildDepth = child.maxDepth
		}
	}

	for i := 0; i < n.paramCount; i++ {
		paramChild := n.paramChildren[i]
		if paramChild != nil && paramChild.maxDepth > maxChildDepth {
			maxChildDepth = paramChild.maxDepth
		}
	}
	if n.wildcardChild != nil && n.wildcardChild.maxDepth > maxChildDepth {
		maxChildDepth = n.wildcardChild.maxDepth
	}
	n.maxDepth = maxChildDepth + 1
}
