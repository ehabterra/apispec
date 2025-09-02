package products

import "github.com/gofiber/fiber/v2"

func Routes(r fiber.Router) {
	r.Get("/", ListProducts)
	r.Post("/", CreateProduct)
	r.Get("/:id", GetProduct)
}
