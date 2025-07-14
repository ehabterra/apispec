package payment

import (
	"github.com/go-chi/chi/v5"
)

// PaymentAPIs creates payment routes
func PaymentAPIs(application interface{}) chi.Router {
	router := chi.NewRouter()

	// Create payment handler (this will be assigned to PaymentHandler interface)
	paymentHandlers := NewPaymentHandler(nil, nil, nil)

	// Apply middleware for all routes
	// router.Use(middleware.TokenStringInContext)

	// Stripe configuration endpoint
	router.Get("/api/v1/stripe/pk", paymentHandlers.GetStripePublicKey())

	// Process payment endpoint
	router.Post("/api/v1/payment/process", paymentHandlers.ProcessPayment())

	return router
}
