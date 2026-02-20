package nanite

import (
	"fmt"
	"net/url"
	"strings"
)

type segmentKind uint8

const (
	segmentStatic segmentKind = iota
	segmentParam
	segmentWildcard
)

type routeSegment struct {
	kind  segmentKind
	value string
}

type namedRoute struct {
	method   string
	path     string
	segments []routeSegment
}

// HandleNamed registers a route and stores metadata for reverse URL generation.
func (r *Router) HandleNamed(name, method, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	if err := r.registerNamedRoute(name, method, path); err != nil {
		panic(err)
	}
	r.addRoute(method, path, handler, middleware...)
	return r
}

func (r *Router) NamedGet(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "GET", path, handler, middleware...)
}

func (r *Router) NamedPost(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "POST", path, handler, middleware...)
}

func (r *Router) NamedPut(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "PUT", path, handler, middleware...)
}

func (r *Router) NamedDelete(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "DELETE", path, handler, middleware...)
}

func (r *Router) NamedPatch(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "PATCH", path, handler, middleware...)
}

func (r *Router) NamedOptions(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "OPTIONS", path, handler, middleware...)
}

func (r *Router) NamedHead(name, path string, handler HandlerFunc, middleware ...MiddlewareFunc) *Router {
	return r.HandleNamed(name, "HEAD", path, handler, middleware...)
}

// URL resolves a named route into a concrete path.
// params keys should match route placeholders (":id" => "id", "*path" => "path").
func (r *Router) URL(name string, params map[string]string) (string, error) {
	r.mutex.RLock()
	route, ok := r.namedRoutes[name]
	r.mutex.RUnlock()
	if !ok {
		return "", fmt.Errorf("named route %q not found", name)
	}

	if len(route.segments) == 0 {
		return "/", nil
	}

	var b strings.Builder
	b.WriteByte('/')
	for i, seg := range route.segments {
		if i > 0 {
			b.WriteByte('/')
		}
		switch seg.kind {
		case segmentStatic:
			b.WriteString(seg.value)
		case segmentParam, segmentWildcard:
			v := ""
			if params != nil {
				v = params[seg.value]
			}
			if v == "" {
				return "", fmt.Errorf("missing parameter %q for route %q", seg.value, name)
			}
			b.WriteString(v)
		}
	}

	return b.String(), nil
}

// MustURL resolves a named route and panics on error.
func (r *Router) MustURL(name string, params map[string]string) string {
	url, err := r.URL(name, params)
	if err != nil {
		panic(err)
	}
	return url
}

// URLWithQuery resolves a named route and appends encoded query parameters.
func (r *Router) URLWithQuery(name string, params map[string]string, query map[string]string) (string, error) {
	base, err := r.URL(name, params)
	if err != nil {
		return "", err
	}
	if len(query) == 0 {
		return base, nil
	}

	values := url.Values{}
	for key, value := range query {
		values.Set(key, value)
	}
	encoded := values.Encode()
	if encoded == "" {
		return base, nil
	}
	return base + "?" + encoded, nil
}

// MustURLWithQuery resolves a named route with query params and panics on error.
func (r *Router) MustURLWithQuery(name string, params map[string]string, query map[string]string) string {
	u, err := r.URLWithQuery(name, params, query)
	if err != nil {
		panic(err)
	}
	return u
}

// NamedRoute returns metadata for a registered named route.
func (r *Router) NamedRoute(name string) (method, path string, ok bool) {
	r.mutex.RLock()
	route, exists := r.namedRoutes[name]
	r.mutex.RUnlock()
	if !exists {
		return "", "", false
	}
	return route.method, route.path, true
}

func (r *Router) registerNamedRoute(name, method, path string) error {
	if name == "" {
		return fmt.Errorf("route name cannot be empty")
	}

	normalizedPath, segments, err := parseRouteTemplate(path)
	if err != nil {
		return err
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()
	if _, exists := r.namedRoutes[name]; exists {
		return fmt.Errorf("named route %q already registered", name)
	}
	r.namedRoutes[name] = namedRoute{
		method:   method,
		path:     normalizedPath,
		segments: segments,
	}
	return nil
}

func parseRouteTemplate(path string) (string, []routeSegment, error) {
	if path == "" {
		path = "/"
	}
	if path[0] != '/' {
		path = "/" + path
	}
	if len(path) > 1 && path[len(path)-1] == '/' {
		path = strings.TrimSuffix(path, "/")
	}

	if path == "/" {
		return path, nil, nil
	}

	parts := strings.Split(path[1:], "/")
	segments := make([]routeSegment, 0, len(parts))
	for i, part := range parts {
		if part == "" {
			return "", nil, fmt.Errorf("invalid route template %q: empty path segment", path)
		}

		switch part[0] {
		case ':':
			name := part[1:]
			if name == "" {
				return "", nil, fmt.Errorf("invalid route template %q: empty param name", path)
			}
			segments = append(segments, routeSegment{kind: segmentParam, value: name})
		case '*':
			name := part[1:]
			if name == "" {
				return "", nil, fmt.Errorf("invalid route template %q: empty wildcard name", path)
			}
			if i != len(parts)-1 {
				return "", nil, fmt.Errorf("invalid route template %q: wildcard must be final segment", path)
			}
			segments = append(segments, routeSegment{kind: segmentWildcard, value: name})
		default:
			segments = append(segments, routeSegment{kind: segmentStatic, value: part})
		}
	}

	return path, segments, nil
}
