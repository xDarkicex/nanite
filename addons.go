package nanite

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

type Group struct {
	middleware []MiddlewareFunc // Middleware stack for all group routes (hot: accessed every request)
	prefix     string           // Path segment prepended to all child routes (e.g., "/api")
	router     *Router          // Reference to root router instance (cold: set at creation)
}

func (r *Router) Group(prefix string, middleware ...MiddlewareFunc) *Group {
	return &Group{
		router:     r,
		prefix:     prefix,
		middleware: middleware,
	}
}

func (g *Group) Get(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Get(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Post(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Post(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Put(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Put(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Delete(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Delete(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Patch(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Patch(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Options(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Options(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Head(path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Head(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Handle(method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.Handle(method, fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) WebSocket(path string, handler WebSocketHandler, middleware ...MiddlewareFunc) *Group {
	fullPath := normalizePath(g.prefix + path)
	allMiddleware := append(g.middleware, middleware...)
	g.router.WebSocket(fullPath, handler, allMiddleware...)
	return g
}

func (g *Group) Group(prefix string, middleware ...MiddlewareFunc) *Group {
	fullPrefix := normalizePath(g.prefix + prefix)
	allMiddleware := append(g.middleware, middleware...)
	return &Group{
		router:     g.router,
		prefix:     fullPrefix,
		middleware: allMiddleware,
	}
}

// - middleware: The middleware functions to add
func (g *Group) Use(middleware ...MiddlewareFunc) *Group {
	g.middleware = append(g.middleware, middleware...)
	return g
}

//go:inline
func normalizePath(path string) string {
	// Fast path for empty string
	if path == "" {
		return "/"
	}

	// Fast path for root path
	if path == "/" {
		return "/"
	}

	// Check if we need to add a leading slash
	needsPrefix := path[0] != '/'

	// Check if we need to remove a trailing slash
	length := len(path)
	needsSuffix := length > 1 && path[length-1] == '/'

	// Fast path: if no changes needed, return original
	if !needsPrefix && !needsSuffix {
		return path
	}

	// Calculate the exact size needed for the new string
	newLen := length
	if needsPrefix {
		newLen++
	}
	if needsSuffix {
		newLen--
	}

	// Create a new string with the exact capacity needed
	var b strings.Builder
	b.Grow(newLen)

	// Add leading slash if needed
	if needsPrefix {
		b.WriteByte('/')
	}

	// Write the path, excluding trailing slash if needed
	if needsSuffix {
		b.WriteString(path[:length-1])
	} else {
		b.WriteString(path)
	}

	return b.String()
}

// ### Validation Middleware

func ValidationMiddleware(chains ...*ValidationChain) MiddlewareFunc {
	return func(ctx *Context, next func()) {
		if ctx.IsAborted() {
			return
		}

		if len(chains) > 0 && (ctx.Request.Method == "POST" || ctx.Request.Method == "PUT" ||
			ctx.Request.Method == "PATCH" || ctx.Request.Method == "DELETE") {
			contentType := ctx.Request.Header.Get("Content-Type")

			if strings.HasPrefix(contentType, "application/x-www-form-urlencoded") ||
				strings.HasPrefix(contentType, "multipart/form-data") {
				if err := ctx.Request.ParseForm(); err != nil {
					ve := getValidationError("", "failed to parse form data")
					if ctx.ValidationErrs == nil {
						ctx.ValidationErrs = make(ValidationErrors, 0, 1)
					}
					ctx.ValidationErrs = append(ctx.ValidationErrs, *ve)
					putValidationError(ve)
					ctx.JSON(http.StatusBadRequest, map[string]interface{}{"errors": ctx.ValidationErrs})
					return
				}

				formData := getMap()
				for key, values := range ctx.Request.Form {
					if len(values) == 1 {
						formData[key] = values[0]
					} else {
						formData[key] = values
					}
				}
				ctx.setPooledMap("formData", formData)
			} else if strings.HasPrefix(contentType, "application/json") {
				originalBody := ctx.Request.Body
				defer func() {
					if err := originalBody.Close(); err != nil {
						// Only handle error if response not yet committed
						if !ctx.IsWritten() {
							ve := getValidationError("", fmt.Sprintf("body closure error: %v", err))
							if ctx.ValidationErrs == nil {
								ctx.ValidationErrs = make(ValidationErrors, 0, 1)
							}
							ctx.ValidationErrs = append(ctx.ValidationErrs, *ve)
							putValidationError(ve)
							ctx.JSON(http.StatusInternalServerError, map[string]interface{}{"errors": ctx.ValidationErrs})
						}
					}
				}()

				maxJSONSize := int64(10 << 20)
				if ctx.Request.ContentLength > maxJSONSize && ctx.Request.ContentLength != -1 {
					ve := getValidationError("", "request body too large")
					if ctx.ValidationErrs == nil {
						ctx.ValidationErrs = make(ValidationErrors, 0, 1)
					}
					ctx.ValidationErrs = append(ctx.ValidationErrs, *ve)
					putValidationError(ve)
					ctx.JSON(http.StatusRequestEntityTooLarge, map[string]interface{}{"errors": ctx.ValidationErrs})
					return
				}

				buffer := bufferPool.Get().(*bytes.Buffer)
				buffer.Reset()
				defer bufferPool.Put(buffer)

				limitedBody := io.LimitReader(originalBody, maxJSONSize)
				teeBody := io.TeeReader(limitedBody, buffer)

				var body map[string]interface{}
				if err := json.NewDecoder(teeBody).Decode(&body); err != nil {
					ve := getValidationError("", "invalid JSON")
					if ctx.ValidationErrs == nil {
						ctx.ValidationErrs = make(ValidationErrors, 0, 1)
					}
					ctx.ValidationErrs = append(ctx.ValidationErrs, *ve)
					putValidationError(ve)
					ctx.JSON(http.StatusBadRequest, map[string]interface{}{"errors": ctx.ValidationErrs})
					return
				}

				ctx.Request.Body = io.NopCloser(bytes.NewReader(buffer.Bytes()))
				ctx.setPooledMap("body", body)
			} else {
				// Unsupported content type - skip validation, continue to handler
				next()
				return
			}

			// Set up validation rules
			for _, chain := range chains {
				field := ctx.Field(chain.field)
				field.rules = append(field.rules, chain.rules...)
			}

			// Run validation BEFORE calling handler
			if !ctx.CheckValidation() {
				// Validation failed, CheckValidation already sent the response
				for _, chain := range chains {
					chain.Release()
				}
				return
			}
		}

		next()

		for _, chain := range chains {
			chain.Release()
		}
	}
}

//go:inline
func executeMiddlewareChain(c *Context, handler HandlerFunc, middleware []MiddlewareFunc) {
	// Fast path for no middleware
	if len(middleware) == 0 {
		handler(c)
		return
	}

	// Handle common cases with direct function calls (no allocations)
	switch len(middleware) {
	case 1:
		middleware[0](c, func() { handler(c) })
		return
	case 2:
		middleware[0](c, func() {
			middleware[1](c, func() { handler(c) })
		})
		return
	case 3:
		middleware[0](c, func() {
			middleware[1](c, func() {
				middleware[2](c, func() { handler(c) })
			})
		})
		return

	case 4:
		middleware[0](c, func() {
			middleware[1](c, func() {
				middleware[2](c, func() {
					middleware[3](c, func() { handler(c) })
				})
			})
		})
		return
	case 5:
		middleware[0](c, func() {
			middleware[1](c, func() {
				middleware[2](c, func() {
					middleware[3](c, func() {
						middleware[4](c, func() { handler(c) })
					})
				})
			})
		})
		return
	}

	// For larger middleware chains, use a completely non-recursive approach
	// that pre-builds all the functions before executing them
	funcs := make([]func(), len(middleware)+1)

	// The last function just calls the handler
	funcs[len(middleware)] = func() {
		handler(c)
	}

	// Build the middleware chain backwards
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		next := funcs[i+1] // Explicitly capture the next function
		funcs[i] = func() {
			mw(c, next)
		}
	}

	// Start the chain execution
	funcs[0]()
}

// ### tracked_response_writer

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
//
//go:inline
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

// ### LRU Implementation

// routeCacheKey uniquely identifies an entry in method/path fallback maps.
type routeCacheKey struct {
	method string
	path   string
}

type entry struct {
	key              routeCacheKey
	handler          HandlerFunc
	params           []Param
	prev             int
	next             int
	hitsSincePromote uint32
}

type lruShard struct {
	capacity int
	maxParams int
	mutex    sync.RWMutex
	entries  []entry
	head     int
	tail     int
	hits     int64
	misses   int64

	getIndices     map[string]int
	postIndices    map[string]int
	putIndices     map[string]int
	deleteIndices  map[string]int
	patchIndices   map[string]int
	headIndices    map[string]int
	optionsIndices map[string]int
	otherIndices   map[routeCacheKey]int

	promoteMask uint32
}

// LRUCache uses sharded LRUs to reduce lock contention on hot request paths.
type LRUCache struct {
	shards    []lruShard
	shardMask uint32
}

// nextPowerOfTwo rounds up to the next power of two.
// This improves performance by aligning with hash table implementation details.
// For example: 10 becomes 16, 120 becomes 128, etc.
func nextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}

	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}

// Define multiple sync.Pools for different parameter slice sizes.
// This reduces GC pressure by reusing parameter slices based on their capacity.
var paramSlicePools = [5]sync.Pool{
	{New: func() interface{} { return make([]Param, 0, 4) }},  // Capacity 4
	{New: func() interface{} { return make([]Param, 0, 8) }},  // Capacity 8
	{New: func() interface{} { return make([]Param, 0, 16) }}, // Capacity 16
	{New: func() interface{} { return make([]Param, 0, 32) }}, // Capacity 32
	{New: func() interface{} { return make([]Param, 0, 64) }}, // Capacity 64
}

// getParamSlice retrieves a parameter slice from the appropriate pool based on paramCount.
// This function optimizes memory usage by selecting a pool with an appropriate capacity
// for the requested number of parameters.
//
//go:inline
func getParamSlice(paramCount int) []Param {
	if paramCount <= 4 {
		return paramSlicePools[0].Get().([]Param)[:0]
	} else if paramCount <= 8 {
		return paramSlicePools[1].Get().([]Param)[:0]
	} else if paramCount <= 16 {
		return paramSlicePools[2].Get().([]Param)[:0]
	} else if paramCount <= 32 {
		return paramSlicePools[3].Get().([]Param)[:0]
	} else {
		return paramSlicePools[4].Get().([]Param)[:0]
	}
}

// putParamSlice returns a parameter slice to the appropriate pool based on its capacity.
// This function recycles parameter slices to reduce garbage collection overhead.
// Slices with capacities that don't match a pool are left for the garbage collector.
//
//go:inline
func putParamSlice(s []Param) {
	cap := cap(s)
	if cap == 4 {
		paramSlicePools[0].Put(s)
	} else if cap == 8 {
		paramSlicePools[1].Put(s)
	} else if cap == 16 {
		paramSlicePools[2].Put(s)
	} else if cap == 32 {
		paramSlicePools[3].Put(s)
	} else if cap == 64 {
		paramSlicePools[4].Put(s)
	}
	// Slices with unexpected capacities are discarded (handled by GC)
}

// Simple string interning for method and path.
// This reduces memory usage by storing only one copy of each unique string.
var stringInterner = struct {
	sync.RWMutex
	m map[string]string
}{
	m: make(map[string]string, 16), // Preallocate for common HTTP methods
}

// internString returns a single canonical instance of the given string.
//
//go:inline
func internString(s string) string {
	stringInterner.RLock()
	if interned, ok := stringInterner.m[s]; ok {
		stringInterner.RUnlock()
		return interned
	}
	stringInterner.RUnlock()

	stringInterner.Lock()
	defer stringInterner.Unlock()
	if interned, ok := stringInterner.m[s]; ok {
		return interned
	}

	stringInterner.m[s] = s // Store the string itself as the canonical copy
	return s
}

const routeCacheShardCount = 16

func makeShard(capacity, maxParams int) lruShard {
	s := lruShard{
		capacity:       capacity,
		maxParams:      maxParams,
		entries:        make([]entry, capacity),
		head:           0,
		tail:           capacity - 1,
		getIndices:     make(map[string]int, capacity),
		postIndices:    make(map[string]int, capacity/8+1),
		putIndices:     make(map[string]int, capacity/8+1),
		deleteIndices:  make(map[string]int, capacity/8+1),
		patchIndices:   make(map[string]int, capacity/16+1),
		headIndices:    make(map[string]int, capacity/16+1),
		optionsIndices: make(map[string]int, capacity/16+1),
		otherIndices:   make(map[routeCacheKey]int, capacity/8+1),
		promoteMask:    7,
	}
	for i := 0; i < capacity; i++ {
		s.entries[i].next = (i + 1) % capacity
		s.entries[i].prev = (i - 1 + capacity) % capacity
	}
	return s
}

// NewLRUCache creates a sharded LRU cache with the specified capacity and maxParams.
func NewLRUCache(capacity, maxParams int) *LRUCache {
	// Set defaults if invalid values provided
	if capacity <= 0 {
		capacity = 1024 // Default size
	}

	// Set a reasonable upper limit to prevent unexpected issues
	if capacity > 16384 {
		capacity = 16384
	}

	// Round capacity to next power of two for better performance
	capacity = nextPowerOfTwo(capacity)
	if maxParams <= 0 {
		maxParams = 10 // Default max parameters
	}

	if maxParams <= 0 {
		maxParams = 10 // Default max parameters
	}
	perShardCapacity := capacity / routeCacheShardCount
	if perShardCapacity < 64 {
		perShardCapacity = 64
	}
	perShardCapacity = nextPowerOfTwo(perShardCapacity)

	c := &LRUCache{
		shards:    make([]lruShard, routeCacheShardCount),
		shardMask: routeCacheShardCount - 1,
	}
	for i := 0; i < routeCacheShardCount; i++ {
		c.shards[i] = makeShard(perShardCapacity, maxParams)
	}
	return c
}

func (s *lruShard) indexMapForMethod(method string) map[string]int {
	switch method {
	case "GET":
		return s.getIndices
	case "POST":
		return s.postIndices
	case "PUT":
		return s.putIndices
	case "DELETE":
		return s.deleteIndices
	case "PATCH":
		return s.patchIndices
	case "HEAD":
		return s.headIndices
	case "OPTIONS":
		return s.optionsIndices
	default:
		return nil
	}
}

func (s *lruShard) getIndex(method, path string) (int, bool) {
	if m := s.indexMapForMethod(method); m != nil {
		idx, ok := m[path]
		return idx, ok
	}
	idx, ok := s.otherIndices[routeCacheKey{method: method, path: path}]
	return idx, ok
}

func (s *lruShard) setIndex(method, path string, idx int) {
	if m := s.indexMapForMethod(method); m != nil {
		m[path] = idx
		return
	}
	s.otherIndices[routeCacheKey{method: method, path: path}] = idx
}

func (s *lruShard) deleteIndex(method, path string) {
	if m := s.indexMapForMethod(method); m != nil {
		delete(m, path)
		return
	}
	delete(s.otherIndices, routeCacheKey{method: method, path: path})
}

//go:inline
func hashRouteKey(method, path string) uint32 {
	// FNV-1a variant tuned for short strings.
	h := uint32(2166136261)
	for i := 0; i < len(method); i++ {
		h ^= uint32(method[i])
		h *= 16777619
	}
	h ^= uint32('/')
	h *= 16777619
	for i := 0; i < len(path); i++ {
		h ^= uint32(path[i])
		h *= 16777619
	}
	return h
}

//go:inline
func (c *LRUCache) shardFor(method, path string) *lruShard {
	return &c.shards[hashRouteKey(method, path)&c.shardMask]
}

func (s *lruShard) Add(method, path string, handler HandlerFunc, params []Param) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// Check if the key already exists
	if idx, exists := s.getIndex(method, path); exists {
		entry := &s.entries[idx]
		if entry.params != nil {
			putParamSlice(entry.params)
		}
		entry.handler = handler
		entry.params = params // Store params directly
		entry.hitsSincePromote = 0
		s.moveToFront(idx)
		return
	}

	idx := s.tail
	entry := &s.entries[idx]
	oldKey := entry.key
	if oldKey.method != "" || oldKey.path != "" {
		s.deleteIndex(oldKey.method, oldKey.path)
	}
	if entry.params != nil {
		putParamSlice(entry.params)
	}

	entry.key = routeCacheKey{method: method, path: path}
	entry.handler = handler
	entry.params = params
	entry.hitsSincePromote = 0
	s.setIndex(method, path, idx)
	s.moveToFront(idx)
}

func (c *LRUCache) Add(method, path string, handler HandlerFunc, params []Param) {
	c.shardFor(method, path).Add(method, path, handler, params)
}

func (s *lruShard) Get(method, path string) (HandlerFunc, []Param, bool) {
	s.mutex.RLock()
	idx, exists := s.getIndex(method, path)
	if !exists {
		atomic.AddInt64(&s.misses, 1)
		s.mutex.RUnlock()
		return nil, nil, false
	}

	entry := &s.entries[idx]
	handler := entry.handler
	params := entry.params // Return cached params directly (caller must not modify)
	promoteMask := atomic.LoadUint32(&s.promoteMask)
	promote := atomic.AddUint32(&entry.hitsSincePromote, 1)&promoteMask == 0

	s.mutex.RUnlock()

	if promote {
		// Sampled promotion keeps LRU quality while reducing write lock pressure.
		s.mutex.Lock()
		if currentIdx, ok := s.getIndex(method, path); ok && currentIdx == idx {
			s.entries[idx].hitsSincePromote = 0
			s.moveToFront(idx)
		}
		s.mutex.Unlock()
	}

	atomic.AddInt64(&s.hits, 1)
	return handler, params, true
}

func (c *LRUCache) Get(method, path string) (HandlerFunc, []Param, bool) {
	return c.shardFor(method, path).Get(method, path)
}

func (s *lruShard) moveToFront(idx int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered from panic in moveToFront: %v\n", r)
		}
	}()

	// Already at front, nothing to do
	if idx == s.head {
		return
	}

	// Safety check for invalid index
	if idx < 0 || idx >= s.capacity {
		fmt.Printf("Warning: Attempted to move invalid index %d in LRU cache with capacity %d\n", idx, s.capacity)
		return
	}

	// Remove from current position
	entry := &s.entries[idx]
	prevIdx := entry.prev
	nextIdx := entry.next
	s.entries[prevIdx].next = nextIdx
	s.entries[nextIdx].prev = prevIdx
	// Update tail if we moved the tail
	if idx == s.tail {
		s.tail = prevIdx
	}

	// Insert at front
	oldHead := s.head
	oldHeadPrev := s.entries[oldHead].prev
	entry.next = oldHead
	entry.prev = oldHeadPrev
	s.entries[oldHead].prev = idx
	s.entries[oldHeadPrev].next = idx
	// Update head
	s.head = idx
}

func (s *lruShard) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for i := range s.entries {
		if s.entries[i].params != nil {
			putParamSlice(s.entries[i].params)
			s.entries[i].params = nil
		}
		s.entries[i].key = routeCacheKey{}
		s.entries[i].handler = nil
	}

	clear(s.getIndices)
	clear(s.postIndices)
	clear(s.putIndices)
	clear(s.deleteIndices)
	clear(s.patchIndices)
	clear(s.headIndices)
	clear(s.optionsIndices)
	clear(s.otherIndices)
	s.head = 0
	s.tail = s.capacity - 1
	for i := 0; i < s.capacity; i++ {
		s.entries[i].next = (i + 1) % s.capacity
		s.entries[i].prev = (i - 1 + s.capacity) % s.capacity
	}

	atomic.StoreInt64(&s.hits, 0)
	atomic.StoreInt64(&s.misses, 0)
}

func (c *LRUCache) Clear() {
	for i := range c.shards {
		c.shards[i].Clear()
	}
}

// SetPromoteEvery configures sampled promotion frequency.
// Value is rounded up to next power of two and clamped to [1, 1024].
func (c *LRUCache) SetPromoteEvery(n uint32) {
	if n == 0 {
		n = 1
	}
	if n > 1024 {
		n = 1024
	}
	// Round up to power of two for mask-based sampling.
	p := uint32(1)
	for p < n {
		p <<= 1
	}
	for i := range c.shards {
		atomic.StoreUint32(&c.shards[i].promoteMask, p-1)
	}
}

func (c *LRUCache) Stats() (hits, misses int64, ratio float64) {
	for i := range c.shards {
		hits += atomic.LoadInt64(&c.shards[i].hits)
		misses += atomic.LoadInt64(&c.shards[i].misses)
	}
	total := hits + misses
	if total > 0 {
		ratio = float64(hits) / float64(total)
	}
	return
}

func (r *Router) SetRouteCacheOptions(size, maxParams int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	// Store settings in config
	if r.config == nil {
		r.config = &Config{}
	}
	r.config.RouteCacheSize = size
	r.config.RouteMaxParams = maxParams
	// Update or disable the cache based on size
	if size <= 0 {
		r.routeCache = nil // Disable caching
	} else {
		r.routeCache = NewLRUCache(size, maxParams)
	}
}

// SetRouteCacheTuning adjusts cache behavior for hit-path contention and usefulness.
// promoteEvery controls sampled hit promotion frequency.
// minDynamicRoutes controls minimum dynamic routes per method before cache is used.
func (r *Router) SetRouteCacheTuning(promoteEvery, minDynamicRoutes int) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if minDynamicRoutes < 0 {
		minDynamicRoutes = 0
	}
	r.routeCacheMinDynamicRoutes = minDynamicRoutes

	if r.routeCache != nil {
		r.routeCache.SetPromoteEvery(uint32(promoteEvery))
	}
}

// SetPanicRecovery controls whether ServeHTTP recovers panics and returns 500s.
// Disabling recovery removes defer/recover overhead from the hot path.
func (r *Router) SetPanicRecovery(enabled bool) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.config == nil {
		r.config = &Config{}
	}
	r.config.RecoverPanics = enabled
}

const (
	DefaultBufferSize = 4096 // Default buffer size for most content types
	TextBufferSize    = 2048 // Smaller buffer size for text-based content for faster flushing
	BinaryBufferSize  = 8192 // Larger buffer size for binary content to minimize syscalls
)

type BufferedResponseWriter struct {
	*TrackedResponseWriter
	buffer     *bytes.Buffer
	bufferSize int
	autoFlush  bool
}

//go:inline
func contentTypeMatch(contentType []byte, prefix []byte) bool {
	if len(contentType) < len(prefix) {
		return false
	}

	for i := 0; i < len(prefix); i++ {
		if contentType[i] != prefix[i] {
			return false
		}
	}
	return true
}

//go:inline
func stripContentParams(contentType []byte) []byte {
	for i := 0; i < len(contentType); i++ {
		if contentType[i] == ';' {
			return contentType[:i]
		}
	}
	return contentType
}

func newBufferedResponseWriter(w *TrackedResponseWriter, contentType string, config *Config) *BufferedResponseWriter {
	// Get buffer from pool and ensure it's empty
	buffer := bufferPool.Get().(*bytes.Buffer)
	buffer.Reset()

	// Convert to byte slice once instead of multiple string operations
	contentTypeBytes := []byte(contentType)

	// Strip content type parameters without allocations
	contentTypeBytes = stripContentParams(contentTypeBytes)

	// Define common content type prefixes as byte slices to avoid repeat conversions
	var textPrefix = []byte("text/")
	var jsonPrefix = []byte("application/json")
	var xmlPrefix = []byte("application/xml")
	var jsPrefix = []byte("application/javascript")
	var formPrefix = []byte("application/x-www-form-urlencoded")

	var imagePrefix = []byte("image/")
	var videoPrefix = []byte("video/")
	var audioPrefix = []byte("audio/")
	var octetPrefix = []byte("application/octet-stream")
	var pdfPrefix = []byte("application/pdf")
	var zipPrefix = []byte("application/zip")

	// Determine buffer size from content type without allocations
	var bufferSize int

	// Fast path for empty content type
	if len(contentTypeBytes) == 0 {
		if config != nil && config.DefaultBufferSize > 0 {
			bufferSize = config.DefaultBufferSize
		} else {
			bufferSize = DefaultBufferSize
		}
	} else if contentTypeMatch(contentTypeBytes, textPrefix) ||
		contentTypeMatch(contentTypeBytes, jsonPrefix) ||
		contentTypeMatch(contentTypeBytes, xmlPrefix) ||
		contentTypeMatch(contentTypeBytes, jsPrefix) ||
		contentTypeMatch(contentTypeBytes, formPrefix) {
		// Text content - use smaller buffer for faster flushing
		if config != nil && config.TextBufferSize > 0 {
			bufferSize = config.TextBufferSize
		} else {
			bufferSize = TextBufferSize
		}
	} else if contentTypeMatch(contentTypeBytes, imagePrefix) ||
		contentTypeMatch(contentTypeBytes, videoPrefix) ||
		contentTypeMatch(contentTypeBytes, audioPrefix) ||
		contentTypeMatch(contentTypeBytes, octetPrefix) ||
		contentTypeMatch(contentTypeBytes, pdfPrefix) ||
		contentTypeMatch(contentTypeBytes, zipPrefix) {
		// Binary content - use larger buffer to minimize syscalls
		if config != nil && config.BinaryBufferSize > 0 {
			bufferSize = config.BinaryBufferSize
		} else {
			bufferSize = BinaryBufferSize
		}
	} else {
		// Default for unknown content types
		if config != nil && config.DefaultBufferSize > 0 {
			bufferSize = config.DefaultBufferSize
		} else {
			bufferSize = DefaultBufferSize
		}
	}

	return &BufferedResponseWriter{
		TrackedResponseWriter: w,
		buffer:                buffer,
		bufferSize:            bufferSize,
		autoFlush:             true,
	}
}

func (w *BufferedResponseWriter) Write(b []byte) (int, error) {
	if !w.headerWritten {
		w.WriteHeader(http.StatusOK)
	}

	// Fast path for small single writes: avoid buffer and content-type logic.
	if w.buffer.Len() == 0 && len(b) <= 1024 {
		return w.TrackedResponseWriter.Write(b)
	}

	// Get current content type after headers are written
	contentType := w.Header().Get("Content-Type")
	isTextBased := strings.HasPrefix(contentType, "text/") ||
		strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "application/xml")

	// For large writes that exceed buffer, write directly to avoid double copying
	if len(b) > w.bufferSize*2 {
		// Flush any existing buffered data first
		if w.buffer.Len() > 0 {
			w.Flush()
		}
		return w.TrackedResponseWriter.Write(b)
	}

	// If this write would exceed buffer size, flush first
	if w.buffer.Len()+len(b) > w.bufferSize {
		w.Flush()
	}

	n, err := w.buffer.Write(b)

	// Adaptive flushing strategy based on content type
	if w.autoFlush {
		bufferFullness := float64(w.buffer.Len()) / float64(w.bufferSize)

		// For text-based content, flush earlier for better perceived performance
		if isTextBased && bufferFullness >= 0.7 {
			w.Flush()
		} else if !isTextBased && w.buffer.Len() >= w.bufferSize {
			// For binary content, wait until buffer is completely full
			w.Flush()
		}
	}

	return n, err
}

// Flush writes buffered data to the underlying ResponseWriter
func (w *BufferedResponseWriter) Flush() {
	if w == nil || w.TrackedResponseWriter == nil || w.buffer == nil {
		return
	}

	if w.buffer.Len() > 0 {
		// Only attempt to write if we have something to write
		w.TrackedResponseWriter.Write(w.buffer.Bytes())
		w.buffer.Reset()
	}
}

func (w *BufferedResponseWriter) Close() {
	if w == nil || w.buffer == nil {
		return
	}

	w.Flush()
	w.buffer.Reset()
}
