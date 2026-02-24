package main

import (
	"fmt"
	"net/http"

	nanite "github.com/xDarkicex/nanite"
)

func main() {
	r := nanite.New()

	// Basic named route
	r.Get("/users/:id", func(c *nanite.Context) {
		id, _ := c.GetParam("id")
		c.String(http.StatusOK, "User: "+id)
	}).Name("user.show")

	// Named route with wildcard
	r.Get("/files/*path", func(c *nanite.Context) {
		path, _ := c.GetParam("path")
		c.String(http.StatusOK, "File: "+path)
	}).Name("files.show")

	// Named routes in groups
	api := r.Group("/api/v1")
	api.Get("/posts/:id", func(c *nanite.Context) {
		id, _ := c.GetParam("id")
		c.JSON(http.StatusOK, map[string]string{"post": id})
	}).Name("api.posts.show")

	api.Post("/posts", func(c *nanite.Context) {
		c.JSON(http.StatusCreated, map[string]string{"status": "created"})
	}).Name("api.posts.create")

	// Generate URLs
	userURL, _ := r.URL("user.show", map[string]string{"id": "123"})
	fmt.Println("User URL:", userURL) // /users/123

	fileURL, _ := r.URL("files.show", map[string]string{"path": "docs/report.pdf"})
	fmt.Println("File URL:", fileURL) // /files/docs/report.pdf

	postURL, _ := r.URL("api.posts.show", map[string]string{"id": "456"})
	fmt.Println("Post URL:", postURL) // /api/v1/posts/456

	// URL with query parameters
	searchURL, _ := r.URLWithQuery("user.show",
		map[string]string{"id": "789"},
		map[string]string{"tab": "profile", "edit": "true"})
	fmt.Println("Search URL:", searchURL) // /users/789?edit=true&tab=profile

	// Get route metadata
	method, path, ok := r.NamedRoute("user.show")
	if ok {
		fmt.Printf("Route metadata: %s %s\n", method, path) // GET /users/:id
	}

	fmt.Println("\nServer would start on :8080")
	// r.Start(":8080")
}
