package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a user in the system
type User struct {
	ID        uuid.UUID  `json:"id" validate:"required"`
	Name      string     `json:"name" validate:"required,min=2,max=50"`
	Email     string     `json:"email" validate:"required,email"`
	Age       int        `json:"age" validate:"min=18,max=120"`
	Status    UserStatus `json:"status" validate:"required"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// UserStatus represents the status of a user
type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
	UserStatusPending  UserStatus = "pending"
)

// CreateUserRequest represents the request payload for creating a user
type CreateUserRequest struct {
	Name  string `json:"name" validate:"required,min=2,max=50"`
	Email string `json:"email" validate:"required,email"`
	Age   int    `json:"age" validate:"min=18,max=120"`
}

// UpdateUserRequest represents the request payload for updating a user
type UpdateUserRequest struct {
	Name   *string     `json:"name,omitempty" validate:"omitempty,min=2,max=50"`
	Email  *string     `json:"email,omitempty" validate:"omitempty,email"`
	Age    *int        `json:"age,omitempty" validate:"omitempty,min=18,max=120"`
	Status *UserStatus `json:"status,omitempty"`
}

// UserListResponse represents the response for listing users
type UserListResponse struct {
	Users      []User     `json:"users"`
	Pagination Pagination `json:"pagination"`
}

// Pagination represents pagination information
type Pagination struct {
	Page       int `json:"page" validate:"min=1"`
	Limit      int `json:"limit" validate:"min=1,max=100"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}
