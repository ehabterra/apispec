package payment

import (
	"github.com/go-chi/chi/v5"
)

// Routes returns a chi.Router with all payment endpoints registered.
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/stripe/pk", GetStripePublicKey)
	r.Post("/payment/process", ProcessPayment)
	return r
}
