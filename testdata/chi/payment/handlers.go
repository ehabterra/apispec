package payment

import (
	"encoding/json"
	"net/http"
)

// PaymentHandler interface
type PaymentHandler interface {
	GetStripePublicKey() http.HandlerFunc
	ProcessPayment() http.HandlerFunc
}

// paymentHandler concrete implementation
type paymentHandler struct {
	logger      interface{}
	config      interface{}
	environment string
	validator   interface{}
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(logger interface{}, config interface{}, validator interface{}) PaymentHandler {
	return &paymentHandler{
		logger:      logger,
		config:      config,
		environment: "production",
		validator:   validator,
	}
}

// GetStripePublicKey returns the Stripe public key
func (h *paymentHandler) GetStripePublicKey() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"public_key": "pk_test_123456789",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// ProcessPayment processes a payment
func (h *paymentHandler) ProcessPayment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"status": "success",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
