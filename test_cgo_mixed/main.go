package main

import (
	"net/http"

	"test_cgo_mixed/ai" // This will have CGO issues
	"test_cgo_mixed/api"
)

func main() {
	// Register API routes
	api.RegisterRoutes()

	// Initialize AI (but this might fail due to CGO)
	ai.Init()

	http.ListenAndServe(":8080", nil)
}
