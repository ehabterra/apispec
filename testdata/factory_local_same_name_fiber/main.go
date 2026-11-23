// Fixture: a fiber app built by a factory and mounted by the caller, where the
// factory's local is named like the caller's variable (issue #628):
//
//	/a     same := Same()       factory local `same`
//	/b     other := Other()     factory local `m` (control)
//	/c     Mount("/c", Same2()) inline
package main

import "github.com/gofiber/fiber/v2"

func h(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

func Same() *fiber.App  { same := fiber.New(); same.Get("/same", h); return same }
func Other() *fiber.App { m := fiber.New(); m.Get("/other", h); return m }
func Same2() *fiber.App { same2 := fiber.New(); same2.Get("/inline", h); return same2 }

func main() {
	app := fiber.New()

	same := Same()
	app.Mount("/a", same)

	other := Other()
	app.Mount("/b", other)

	app.Mount("/c", Same2())

	_ = app.Listen(":8080")
}
