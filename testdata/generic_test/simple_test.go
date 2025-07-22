package main

import (
	"net/http"
)

// Simple generic function for testing
func SimpleGeneric[T any](data T) T {
	return data
}

// Simple handler for testing
func simpleHandler(w http.ResponseWriter, r *http.Request) {
	// This should be detected by the pattern matcher
	SimpleGeneric[string]("test")
}
