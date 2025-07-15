package products

import "github.com/gofiber/fiber/v2"

func Routes() *fiber.App {
	r := fiber.New()
	r.Get("/", ListProducts)
	r.Post("/", CreateProduct)
	r.Get("/:id", GetProduct)
	return r
}
