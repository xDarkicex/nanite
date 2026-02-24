package nanite

// Route represents a registered route and provides methods for naming and configuration.
type Route struct {
	method string
	path   string
	router *Router
}

// Name assigns a name to this route for reverse URL generation.
// Returns the router to allow continued chaining of route registrations.
func (rt *Route) Name(name string) *Router {
	if err := rt.router.registerNamedRoute(name, rt.method, rt.path); err != nil {
		panic(err)
	}
	return rt.router
}
