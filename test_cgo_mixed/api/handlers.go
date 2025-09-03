package api

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	response := Response{
		Status:  "ok",
		Message: "API is healthy",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func StatusHandler(w http.ResponseWriter, r *http.Request) {
	response := Response{
		Status:  "active",
		Message: "Service is running",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func RegisterRoutes() {
	http.HandleFunc("/health", HealthHandler)
	http.HandleFunc("/api/status", StatusHandler)
}
