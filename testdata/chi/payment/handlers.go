package payment

import (
	"encoding/json"
	"net/http"
)

// GetStripePublicKey returns the Stripe public key for the payment system.
func GetStripePublicKey(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"public_key": "pk_test_123456789",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ProcessPayment processes a payment request.
func ProcessPayment(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "success",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
