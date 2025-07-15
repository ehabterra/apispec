package users

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

var users = []User{
	{ID: 1, Name: "John Doe", Email: "john@example.com"},
	{ID: 2, Name: "Jane Smith", Email: "jane@example.com"},
}

func ListUsers(c *fiber.Ctx) error {
	return c.JSON(users)
}

func GetUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}
	for _, user := range users {
		if user.ID == id {
			return c.JSON(user)
		}
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
}

func CreateUser(c *fiber.Ctx) error {
	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Name is required"})
	}
	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Email is required"})
	}
	newUser := User{ID: len(users) + 1, Name: req.Name, Email: req.Email}
	users = append(users, newUser)
	return c.Status(fiber.StatusCreated).JSON(newUser)
}

func UpdateUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}
	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}
	for i, user := range users {
		if user.ID == id {
			users[i].Name = req.Name
			users[i].Email = req.Email
			return c.JSON(users[i])
		}
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
}

func DeleteUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID"})
	}
	for i, user := range users {
		if user.ID == id {
			users = append(users[:i], users[i+1:]...)
			return c.JSON(fiber.Map{"message": "User deleted successfully"})
		}
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
}
