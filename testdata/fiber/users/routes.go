package users

import "github.com/gofiber/fiber/v2"

func Routes() *fiber.App {
	r := fiber.New()
	r.Get("/", ListUsers)
	r.Get("/:id", GetUser)
	r.Post("/", CreateUser)
	r.Put("/:id", UpdateUser)
	r.Delete("/:id", DeleteUser)
	return r
}
