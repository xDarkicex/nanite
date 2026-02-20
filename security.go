package nanite

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimitConfig controls opt-in request rate limiting middleware behavior.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration

	// KeyFunc returns the bucket key for the current request.
	// Default uses best-effort client IP.
	KeyFunc func(*Context) string

	StatusCode int
	Message    string
	OnLimit    func(*Context)
}

// DefaultRateLimitConfig returns conservative default settings.
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Requests:   60,
		Window:     time.Minute,
		KeyFunc:    defaultRateLimitKey,
		StatusCode: http.StatusTooManyRequests,
		Message:    "Too Many Requests",
	}
}

// RateLimitMiddleware applies fixed-window per-key request limiting.
func RateLimitMiddleware(cfg *RateLimitConfig) MiddlewareFunc {
	conf := DefaultRateLimitConfig()
	if cfg != nil {
		if cfg.Requests > 0 {
			conf.Requests = cfg.Requests
		}
		if cfg.Window > 0 {
			conf.Window = cfg.Window
		}
		if cfg.KeyFunc != nil {
			conf.KeyFunc = cfg.KeyFunc
		}
		if cfg.StatusCode > 0 {
			conf.StatusCode = cfg.StatusCode
		}
		if cfg.Message != "" {
			conf.Message = cfg.Message
		}
		conf.OnLimit = cfg.OnLimit
	}

	type bucket struct {
		count int
		reset time.Time
	}

	var (
		mu      sync.Mutex
		buckets = make(map[string]bucket)
		tick    uint32
	)

	cleanup := func(now time.Time) {
		for k, b := range buckets {
			if now.After(b.reset) {
				delete(buckets, k)
			}
		}
	}

	return func(c *Context, next func()) {
		key := conf.KeyFunc(c)
		if key == "" {
			key = "_default"
		}

		now := time.Now()
		limited := false

		mu.Lock()
		b := buckets[key]
		if b.reset.IsZero() || now.After(b.reset) {
			b.count = 1
			b.reset = now.Add(conf.Window)
		} else {
			b.count++
		}
		if b.count > conf.Requests {
			limited = true
		}
		buckets[key] = b

		if atomic.AddUint32(&tick, 1)&1023 == 0 {
			cleanup(now)
		}
		mu.Unlock()

		if limited {
			if conf.OnLimit != nil {
				conf.OnLimit(c)
			} else {
				c.String(conf.StatusCode, conf.Message)
			}
			return
		}

		next()
	}
}

// CSRFConfig controls CSRF middleware behavior.
type CSRFConfig struct {
	CookieName string
	HeaderName string
	FormField  string

	CookiePath     string
	CookieDomain   string
	CookieMaxAge   int
	CookieSecure   bool
	CookieHTTPOnly bool
	CookieSameSite http.SameSite

	TokenLength int

	// UnsafeMethods are protected. Missing map uses POST/PUT/PATCH/DELETE.
	UnsafeMethods map[string]struct{}

	ErrorStatus  int
	ErrorMessage string
	OnError      func(*Context)
}

// DefaultCSRFConfig returns a baseline config suitable for most apps.
func DefaultCSRFConfig() CSRFConfig {
	return CSRFConfig{
		CookieName:     "nanite_csrf",
		HeaderName:     "X-CSRF-Token",
		FormField:      "csrf_token",
		CookiePath:     "/",
		CookieMaxAge:   86400,
		CookieHTTPOnly: true,
		CookieSameSite: http.SameSiteLaxMode,
		TokenLength:    32,
		UnsafeMethods:  defaultUnsafeMethods(),
		ErrorStatus:    http.StatusForbidden,
		ErrorMessage:   "invalid CSRF token",
		CookieSecure:   false,
		CookieDomain:   "",
	}
}

// CSRFMiddleware validates CSRF token on unsafe methods and sets token cookie on safe methods.
func CSRFMiddleware(cfg *CSRFConfig) MiddlewareFunc {
	conf := DefaultCSRFConfig()
	if cfg != nil {
		if cfg.CookieName != "" {
			conf.CookieName = cfg.CookieName
		}
		if cfg.HeaderName != "" {
			conf.HeaderName = cfg.HeaderName
		}
		if cfg.FormField != "" {
			conf.FormField = cfg.FormField
		}
		if cfg.CookiePath != "" {
			conf.CookiePath = cfg.CookiePath
		}
		if cfg.CookieDomain != "" {
			conf.CookieDomain = cfg.CookieDomain
		}
		if cfg.CookieMaxAge != 0 {
			conf.CookieMaxAge = cfg.CookieMaxAge
		}
		conf.CookieSecure = cfg.CookieSecure
		conf.CookieHTTPOnly = cfg.CookieHTTPOnly
		if cfg.CookieSameSite != 0 {
			conf.CookieSameSite = cfg.CookieSameSite
		}
		if cfg.TokenLength > 0 {
			conf.TokenLength = cfg.TokenLength
		}
		if cfg.UnsafeMethods != nil {
			conf.UnsafeMethods = cfg.UnsafeMethods
		}
		if cfg.ErrorStatus > 0 {
			conf.ErrorStatus = cfg.ErrorStatus
		}
		if cfg.ErrorMessage != "" {
			conf.ErrorMessage = cfg.ErrorMessage
		}
		conf.OnError = cfg.OnError
	}

	writeTokenCookie := func(c *Context, token string) {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     conf.CookieName,
			Value:    token,
			Path:     conf.CookiePath,
			Domain:   conf.CookieDomain,
			MaxAge:   conf.CookieMaxAge,
			HttpOnly: conf.CookieHTTPOnly,
			Secure:   conf.CookieSecure,
			SameSite: conf.CookieSameSite,
		})
		c.SetHeader(conf.HeaderName, token)
	}

	deny := func(c *Context) {
		if conf.OnError != nil {
			conf.OnError(c)
		} else {
			c.String(conf.ErrorStatus, conf.ErrorMessage)
		}
	}

	return func(c *Context, next func()) {
		method := c.Request.Method
		_, protect := conf.UnsafeMethods[method]

		cookie, _ := c.Request.Cookie(conf.CookieName)
		cookieToken := ""
		if cookie != nil {
			cookieToken = cookie.Value
		}

		if !protect {
			if cookieToken == "" {
				token, err := generateCSRFToken(conf.TokenLength)
				if err == nil {
					writeTokenCookie(c, token)
				}
			} else {
				c.SetHeader(conf.HeaderName, cookieToken)
			}
			next()
			return
		}

		if cookieToken == "" {
			deny(c)
			return
		}

		provided := c.Request.Header.Get(conf.HeaderName)
		if provided == "" {
			provided = c.Request.FormValue(conf.FormField)
		}
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(cookieToken)) != 1 {
			deny(c)
			return
		}

		next()
	}
}

func defaultUnsafeMethods() map[string]struct{} {
	return map[string]struct{}{
		http.MethodPost:   {},
		http.MethodPut:    {},
		http.MethodPatch:  {},
		http.MethodDelete: {},
	}
}

func defaultRateLimitKey(c *Context) string {
	if ip := c.Request.Header.Get("X-Forwarded-For"); ip != "" {
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return stringsTrimSpace(ip[:i])
			}
		}
		return stringsTrimSpace(ip)
	}

	if ip := c.Request.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err == nil {
		return host
	}
	return c.Request.RemoteAddr
}

func generateCSRFToken(tokenLen int) (string, error) {
	if tokenLen <= 0 {
		tokenLen = 32
	}
	b := make([]byte, tokenLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func stringsTrimSpace(s string) string {
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

// ParseRateLimitWindow parses duration shorthand used in external config bridges.
func ParseRateLimitWindow(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// ParseRateLimitRequests parses request limits from string config values.
func ParseRateLimitRequests(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
