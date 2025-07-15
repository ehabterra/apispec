package products

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

var products = []Product{
	{ID: 1, Name: "Widget", Price: 9.99},
	{ID: 2, Name: "Gadget", Price: 19.99},
}

func ListProducts(c *fiber.Ctx) error {
	return c.JSON(products)
}

func GetProduct(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid product ID"})
	}
	for _, product := range products {
		if product.ID == id {
			return c.JSON(product)
		}
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Product not found"})
}

func CreateProduct(c *fiber.Ctx) error {
	var req CreateProductRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	newProduct := Product{ID: len(products) + 1, Name: req.Name, Price: req.Price}
	products = append(products, newProduct)
	return c.Status(fiber.StatusCreated).JSON(newProduct)
}
