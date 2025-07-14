package products

import (
	"encoding/json"
	"net/http"
)

func ListProducts(w http.ResponseWriter, r *http.Request) {
	products := []Product{
		{ID: "1", Name: "Widget", Price: 9.99},
		{ID: "2", Name: "Gadget", Price: 19.99},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(products)
}

func CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	product := Product{ID: "3", Name: req.Name, Price: req.Price}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(product)
}

func GetProduct(w http.ResponseWriter, r *http.Request) {
	product := Product{ID: "1", Name: "Widget", Price: 9.99}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(product)
}
