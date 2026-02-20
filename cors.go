package nanite

import (
	"net/http"
	"strconv"
	"strings"
)

// CORSConfig controls CORSMiddleware behavior.
type CORSConfig struct {
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	ExposeHeaders    []string
	AllowCredentials bool
	MaxAge           int
	AllowOriginFunc  func(origin string) bool
}

// DefaultCORSConfig returns secure default CORS settings.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders: []string{"Content-Type", "Authorization"},
		MaxAge:       600,
	}
}

// CORSMiddleware sets CORS headers and short-circuits valid preflight requests.
func CORSMiddleware(cfg *CORSConfig) MiddlewareFunc {
	conf := DefaultCORSConfig()
	if cfg != nil {
		if len(cfg.AllowOrigins) > 0 {
			conf.AllowOrigins = append([]string(nil), cfg.AllowOrigins...)
		}
		if len(cfg.AllowMethods) > 0 {
			conf.AllowMethods = append([]string(nil), cfg.AllowMethods...)
		}
		if len(cfg.AllowHeaders) > 0 {
			conf.AllowHeaders = append([]string(nil), cfg.AllowHeaders...)
		}
		if len(cfg.ExposeHeaders) > 0 {
			conf.ExposeHeaders = append([]string(nil), cfg.ExposeHeaders...)
		}
		conf.AllowCredentials = cfg.AllowCredentials
		if cfg.MaxAge >= 0 {
			conf.MaxAge = cfg.MaxAge
		}
		conf.AllowOriginFunc = cfg.AllowOriginFunc
	}

	allowMethods := strings.Join(conf.AllowMethods, ", ")
	allowHeaders := strings.Join(conf.AllowHeaders, ", ")
	exposeHeaders := strings.Join(conf.ExposeHeaders, ", ")
	allowAllOrigins := len(conf.AllowOrigins) == 1 && conf.AllowOrigins[0] == "*"

	return func(c *Context, next func()) {
		origin := c.Request.Header.Get("Origin")
		allowedOrigin := ""

		if origin != "" {
			if conf.AllowOriginFunc != nil {
				if conf.AllowOriginFunc(origin) {
					allowedOrigin = origin
				}
			} else if allowAllOrigins {
				if conf.AllowCredentials {
					allowedOrigin = origin
				} else {
					allowedOrigin = "*"
				}
			} else {
				for _, candidate := range conf.AllowOrigins {
					if candidate == origin {
						allowedOrigin = origin
						break
					}
				}
			}
		}

		if allowedOrigin != "" {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", allowedOrigin)
			h.Add("Vary", "Origin")
			if conf.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if exposeHeaders != "" {
				h.Set("Access-Control-Expose-Headers", exposeHeaders)
			}
		}

		isPreflight := c.Request.Method == http.MethodOptions && c.Request.Header.Get("Access-Control-Request-Method") != ""
		if isPreflight {
			h := c.Writer.Header()
			if allowMethods != "" {
				h.Set("Access-Control-Allow-Methods", allowMethods)
			}
			if allowHeaders != "" {
				h.Set("Access-Control-Allow-Headers", allowHeaders)
			}
			if conf.MaxAge > 0 {
				h.Set("Access-Control-Max-Age", strconv.Itoa(conf.MaxAge))
			}
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}

		next()
	}
}
