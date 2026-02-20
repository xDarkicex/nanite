package quic

import (
	"fmt"
	"net/http"
	"time"
)

// AltSvcConfig controls generation of Alt-Svc response headers.
type AltSvcConfig struct {
	// UDPPort is the advertised HTTP/3 UDP port (for h3=":<port>").
	UDPPort int
	// MaxAge controls Alt-Svc cache duration in seconds.
	MaxAge int
}

// DefaultAltSvcConfig returns sane Alt-Svc defaults.
func DefaultAltSvcConfig() AltSvcConfig {
	return AltSvcConfig{
		UDPPort: 8443,
		MaxAge:  86400,
	}
}

// HeaderValue returns an Alt-Svc header value for HTTP/3 advertisement.
func (c AltSvcConfig) HeaderValue() string {
	port := c.UDPPort
	if port <= 0 {
		port = 8443
	}
	ma := c.MaxAge
	if ma <= 0 {
		ma = 86400
	}
	return fmt.Sprintf("h3=\":%d\"; ma=%d", port, ma)
}

// AltSvcMiddleware adds Alt-Svc header on all responses for net/http handlers.
func AltSvcMiddleware(cfg AltSvcConfig) func(http.Handler) http.Handler {
	value := cfg.HeaderValue()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Alt-Svc", value)
			next.ServeHTTP(w, r)
		})
	}
}

// AltSvcHeaderValue returns an Alt-Svc header using a duration max-age.
func AltSvcHeaderValue(udpPort int, maxAge time.Duration) string {
	seconds := int(maxAge / time.Second)
	if seconds <= 0 {
		seconds = 86400
	}
	cfg := AltSvcConfig{UDPPort: udpPort, MaxAge: seconds}
	return cfg.HeaderValue()
}

// AltSvcHeaderValueSeconds returns an Alt-Svc header with max-age in seconds.
func AltSvcHeaderValueSeconds(udpPort int, maxAgeSeconds int) string {
	cfg := AltSvcConfig{UDPPort: udpPort, MaxAge: maxAgeSeconds}
	return cfg.HeaderValue()
}
