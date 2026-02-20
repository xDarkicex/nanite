package quic

import "github.com/xDarkicex/nanite"

// NewRouterServer constructs a QUIC server from a Nanite router.
func NewRouterServer(router *nanite.Router, cfg Config) *Server {
	return New(router, cfg)
}

// StartRouterHTTP3 starts HTTP/3 for a Nanite router.
func StartRouterHTTP3(router *nanite.Router, cfg Config) error {
	return NewRouterServer(router, cfg).StartHTTP3()
}

// StartRouterDual starts dual-stack HTTP/1 + HTTP/3 for a Nanite router.
func StartRouterDual(router *nanite.Router, cfg Config) error {
	return NewRouterServer(router, cfg).StartDual()
}

// AltSvcNaniteMiddleware sets Alt-Svc header for Nanite middleware chains.
func AltSvcNaniteMiddleware(cfg AltSvcConfig) nanite.MiddlewareFunc {
	value := cfg.HeaderValue()
	return func(c *nanite.Context, next func()) {
		c.SetHeader("Alt-Svc", value)
		next()
	}
}
