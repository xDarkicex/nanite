package nanite

import (
	"time"
)

// Option configures router behavior at construction time.
type Option func(*Router)

// WithPanicRecovery enables or disables panic recovery in ServeHTTP.
func WithPanicRecovery(enabled bool) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.RecoverPanics = enabled
	}
}

// WithRouteCacheOptions configures route cache size and max params.
func WithRouteCacheOptions(size, maxParams int) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.RouteCacheSize = size
		r.config.RouteMaxParams = maxParams
	}
}

// WithRouteCacheTuning configures sampled promotion and cache activation threshold.
func WithRouteCacheTuning(promoteEvery uint32, minDynamicRoutes int) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.RouteCachePromote = promoteEvery
		r.config.RouteCacheMinDyn = minDynamicRoutes
	}
}

// WithServerTimeouts configures net/http server timeout values.
func WithServerTimeouts(read, write, idle time.Duration) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.ServerReadTimeout = read
		r.config.ServerWriteTimeout = write
		r.config.ServerIdleTimeout = idle
	}
}

// WithServerMaxHeaderBytes configures max request header size.
func WithServerMaxHeaderBytes(maxHeaderBytes int) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.ServerMaxHeaderBytes = maxHeaderBytes
	}
}

// WithTCPBufferSizes configures accepted TCP connection read/write buffers.
func WithTCPBufferSizes(readBuffer, writeBuffer int) Option {
	return func(r *Router) {
		if r.config == nil {
			r.config = &Config{}
		}
		r.config.TCPReadBuffer = readBuffer
		r.config.TCPWriteBuffer = writeBuffer
	}
}

// WithConfig replaces router configuration with a caller-provided config.
func WithConfig(cfg Config) Option {
	return func(r *Router) {
		copy := cfg
		r.config = &copy
	}
}
