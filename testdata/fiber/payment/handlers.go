package payment

import "github.com/gofiber/fiber/v2"

func GetStripePublicKey(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"public_key": "pk_test_123456789"})
}

func ProcessPayment(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "success"})
}
