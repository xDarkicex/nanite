package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/xDarkicex/nanite"
)

func main() {
	r := nanite.New(nanite.WithPanicRecovery(true))

	// Register error middleware with proper error inspection
	r.UseError(func(err error, c *nanite.Context, next func()) {
		log.Printf("Error occurred: %v", err)

		// Inspect error to return appropriate status code
		errMsg := err.Error()
		if strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "authorization") {
			c.JSON(401, map[string]string{"error": "Unauthorized"})
		} else if strings.Contains(errMsg, "not found") {
			c.JSON(404, map[string]string{"error": "Not found"})
		} else if strings.Contains(errMsg, "too large") || strings.Contains(errMsg, "invalid") {
			c.JSON(400, map[string]string{"error": errMsg})
		} else {
			c.JSON(500, map[string]string{"error": "Internal server error"})
		}
	})

	// Example 1: Route-specific middleware that validates file size
	sizeLimitMiddleware := func(c *nanite.Context, next func()) {
		maxSize := int64(1024 * 1024) // 1MB
		if c.Request.ContentLength > maxSize {
			c.Error(errors.New("file too large"))
			return
		}
		next()
	}

	r.Post("/upload", func(c *nanite.Context) {
		c.JSON(200, map[string]string{"status": "uploaded"})
	}, sizeLimitMiddleware)

	// Example 2: Global auth middleware
	authMiddleware := func(c *nanite.Context, next func()) {
		token := c.Request.Header.Get("Authorization")
		if token == "" {
			c.Error(errors.New("missing authorization header"))
			return
		}
		if token != "Bearer valid-token" {
			c.Error(errors.New("unauthorized: invalid token"))
			return
		}
		next()
	}

	// Protected routes
	api := r.Group("/api", authMiddleware)
	
	api.Get("/users/:id", func(c *nanite.Context) {
		id := c.MustParam("id")
		
		// Simulate database lookup
		if id == "999" {
			c.Error(errors.New("user not found"))
			return
		}
		
		c.JSON(200, map[string]string{
			"id":   id,
			"name": "John Doe",
		})
	})

	api.Post("/users", func(c *nanite.Context) {
		var user map[string]interface{}
		if err := c.Bind(&user); err != nil {
			c.Error(fmt.Errorf("invalid request body: %w", err))
			return
		}
		
		c.JSON(201, map[string]string{
			"status": "created",
			"id":     "123",
		})
	})

	// Example 3: Handler that may panic (caught by panic recovery)
	r.Get("/panic", func(c *nanite.Context) {
		panic("something went wrong!")
	})

	fmt.Println("Server starting on :8080")
	fmt.Println("\nTry these requests:")
	fmt.Println("  curl -X POST http://localhost:8080/upload -H 'Content-Length: 2000000'")
	fmt.Println("  curl http://localhost:8080/api/users/123")
	fmt.Println("  curl http://localhost:8080/api/users/123 -H 'Authorization: Bearer valid-token'")
	fmt.Println("  curl http://localhost:8080/api/users/999 -H 'Authorization: Bearer valid-token'")
	fmt.Println("  curl http://localhost:8080/panic")
	
	r.Start("8080")
}
