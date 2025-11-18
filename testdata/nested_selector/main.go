package main

import (
	"encoding/json"
	"net/http"
)

// Service represents a service with nested client
type Service struct {
	client *Client
}

// Client represents an HTTP client
type Client struct {
	http *http.Client
}

// Handler represents a handler with a function field
type Handler struct {
	process func(string) string
}

// Config represents a configuration with nested services
type Config struct {
	service *Service
}

// Request represents an HTTP request
type Request struct {
	Method string
	URL    string
}

// Response represents an HTTP response
type Response struct {
	Status int
	Body   string
}

// GetService returns a service instance
func GetService() *Service {
	return &Service{
		client: &Client{
			http: &http.Client{},
		},
	}
}

// GetHandler returns a handler instance
func GetHandler() *Handler {
	return &Handler{
		process: func(s string) string {
			return "processed: " + s
		},
	}
}

// GetConfig returns a config instance
func GetConfig() *Config {
	return &Config{
		service: GetService(),
	}
}

// Example 1: Nested Selector - svc.client.http
// This demonstrates the nested selector case where:
// - arg.X is a selector (svc.client)
// - arg.X.Sel is an identifier (client)
// - arg.X.X is the base variable (svc)
func ProcessRequest(svc *Service) {
	// Nested selector: svc.client.http
	// The tracker should trace back to 'svc' and link to its origin
	_ = svc.client.http
}

// Example 2: Function Type Selector - handler.process
// This demonstrates the function type selector case where:
// - arg.Sel.GetType() starts with "func("
// - The tracker should trace back to 'handler' and link to its origin
func ProcessWithHandler(handler *Handler, input string) string {
	// Function type selector: handler.process
	// The tracker should trace back to 'handler' and link to its origin
	return handler.process(input)
}

// Example 3: Complex Nested Case - config.service.client.http
// This demonstrates a triple nested selector case
func ProcessWithConfig(config *Config) {
	// Triple nested: config.service.client.http
	// The tracker should:
	// 1. Detect nested selector (config.service.client)
	// 2. Trace back to 'config' origin
	// 3. Link to config's assignment/parameter
	_ = config.service.client.http
}

// Example 4: Nested Selector with Assignment
// This shows how the tracker links to assignments
func ProcessWithAssignedService() {
	// Assignment: svc is assigned from GetService()
	svc := GetService()

	// Nested selector: svc.client.http
	// The tracker should link this to the assignment above
	_ = svc.client.http
}

// Example 5: Function Type Selector with Assignment
// This shows how the tracker links function type selectors to assignments
func ProcessWithAssignedHandler() {
	// Assignment: handler is assigned from GetHandler()
	handler := GetHandler()

	// Function type selector: handler.process
	// The tracker should link this to the assignment above
	_ = handler.process("test")
}

// Example 6: Generic Function Type Selector
// This demonstrates generic function types (func[T any](T) T)
// Note: Go doesn't support generic function types as struct fields directly,
// so we use a type alias to demonstrate the concept
type TransformFunc[T any] func(T) T

type GenericHandler struct {
	transform TransformFunc[int]
}

func ProcessGeneric(handler *GenericHandler, value int) int {
	// Generic function type selector: handler.transform
	// The tracker should handle func[ prefix (if the type string includes it)
	return handler.transform(value)
}

// HTTP Handlers

// ServiceHandler handles HTTP requests using nested selectors
type ServiceHandler struct{}

type SuccessResponse struct {
	Status string `json:"status"`
}

// GetServiceHandler demonstrates nested selector in HTTP handler
func (h *ServiceHandler) GetServiceHandler(w http.ResponseWriter, r *http.Request) {
	// Get service from parameter (will be traced)
	svc := GetService()

	// Nested selector: svc.client.http
	// This should be linked to the svc variable above
	client := svc.client.http

	// Use the client
	_ = client

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&SuccessResponse{Status: "ok"})
}

// HandlerHandler demonstrates function type selector in HTTP handler
func (h *ServiceHandler) HandlerHandler(w http.ResponseWriter, r *http.Request) {
	// Get handler from parameter (will be traced)
	handler := GetHandler()

	// Function type selector: handler.process
	// This should be linked to the handler variable above
	result := handler.process("input")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

// ConfigHandler demonstrates complex nested selector in HTTP handler
func (h *ServiceHandler) ConfigHandler(w http.ResponseWriter, r *http.Request) {
	// Get config from parameter (will be traced)
	config := GetConfig()

	// Triple nested: config.service.client.http
	// This should be linked to the config variable above
	client := config.service.client.http

	// Use the client
	_ = client

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	handler := &ServiceHandler{}
	http.HandleFunc("/service", handler.GetServiceHandler)
	http.HandleFunc("/handler", handler.HandlerHandler)
	http.HandleFunc("/config", handler.ConfigHandler)
	http.ListenAndServe(":8080", nil)
}
