package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

// APIModule represents an API module that can mount routes
type APIModule interface {
	GetModuleName() string
	RegisterRoutes(router *mux.Router)
}

// Server represents an HTTP server with API modules
type Server struct {
	router  *mux.Router
	modules []APIModule
}

// ProductModule represents a product API module
type ProductModule struct{}

func (m *ProductModule) GetModuleName() string {
	return "product"
}

func (m *ProductModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/products", m.ListProducts).Methods("GET")
	router.HandleFunc("/products/{id}", m.GetProduct).Methods("GET")
}

func (m *ProductModule) ListProducts(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *ProductModule) GetProduct(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// InventoryModule represents an inventory API module
type InventoryModule struct{}

func (m *InventoryModule) GetModuleName() string {
	return "inventory"
}

func (m *InventoryModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/inventory", m.GetInventory).Methods("GET")
	router.HandleFunc("/inventory/stock", m.GetStock).Methods("GET")
}

func (m *InventoryModule) GetInventory(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *InventoryModule) GetStock(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ShippingModule represents a shipping API module
type ShippingModule struct{}

func (m *ShippingModule) GetModuleName() string {
	return "shipping"
}

func (m *ShippingModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/shipping/rates", m.GetRates).Methods("GET")
	router.HandleFunc("/shipping/track/{id}", m.TrackShipment).Methods("GET")
}

func (m *ShippingModule) GetRates(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *ShippingModule) TrackShipment(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// NotificationModule represents a notification API module
type NotificationModule struct{}

func (m *NotificationModule) GetModuleName() string {
	return "notification"
}

func (m *NotificationModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/notifications", m.ListNotifications).Methods("GET")
	router.HandleFunc("/notifications/send", m.SendNotification).Methods("POST")
}

func (m *NotificationModule) ListNotifications(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *NotificationModule) SendNotification(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// AnalyticsModule represents an analytics API module
type AnalyticsModule struct{}

func (m *AnalyticsModule) GetModuleName() string {
	return "analytics"
}

func (m *AnalyticsModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/analytics/reports", m.GetReports).Methods("GET")
	router.HandleFunc("/analytics/metrics", m.GetMetrics).Methods("GET")
}

func (m *AnalyticsModule) GetReports(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *AnalyticsModule) GetMetrics(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// CacheModule represents a cache API module
type CacheModule struct{}

func (m *CacheModule) GetModuleName() string {
	return "cache"
}

func (m *CacheModule) RegisterRoutes(router *mux.Router) {
	router.HandleFunc("/cache/stats", m.GetStats).Methods("GET")
	router.HandleFunc("/cache/clear", m.ClearCache).Methods("POST")
}

func (m *CacheModule) GetStats(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (m *CacheModule) ClearCache(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// MountModule returns a function that mounts an API module to the server
// This is the functional options pattern: MountModule(module) returns func(*Server)
// The IoC relationship here is critical: the module assignment is linked to its usage
// in the server initialization, which affects route registration and URL mounting.
func MountModule(module APIModule) func(*Server) {
	return func(srv *Server) {
		srv.modules = append(srv.modules, module)
		// Register routes when module is mounted
		module.RegisterRoutes(srv.router)
	}
}

// NewServer creates a new server with the given options
func NewServer(opts ...func(*Server)) *Server {
	srv := &Server{
		router:  mux.NewRouter(),
		modules: make([]APIModule, 0),
	}

	for _, opt := range opts {
		opt(srv)
	}

	return srv
}

// InitializeServer creates a new server with all API modules mounted
// This demonstrates the functional options pattern where multiple MountModule calls
// are passed to NewServer, and each MountModule call creates a function that
// modifies the server and registers routes. The tracker should link the assignment
// of each module variable to the argument node where it's used, which is crucial
// for understanding the API structure and route mounting hierarchy.
func InitializeServer() *Server {
	productMod := &ProductModule{}
	inventoryMod := &InventoryModule{}
	shippingMod := &ShippingModule{}
	notificationMod := &NotificationModule{}
	analyticsMod := &AnalyticsModule{}
	cacheMod := &CacheModule{}

	srv := NewServer(
		MountModule(productMod),
		MountModule(inventoryMod),
		MountModule(shippingMod),
		MountModule(notificationMod),
		MountModule(analyticsMod),
		MountModule(cacheMod),
	)

	return srv
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

// HealthHandler handles health check requests
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	srv := InitializeServer()

	// Add a health check endpoint
	srv.router.HandleFunc("/health", HealthHandler).Methods("GET")

	http.ListenAndServe(":8080", srv.router)
}
