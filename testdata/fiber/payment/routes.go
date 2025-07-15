package payment

import "github.com/gofiber/fiber/v2"

func Routes() *fiber.App {
	r := fiber.New()
	r.Get("/stripe/pk", GetStripePublicKey)
	r.Post("/payment/process", ProcessPayment)
	return r
}
