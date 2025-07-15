package main

// User represents a user in the system.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// CreateUserRequest is the payload for creating a new user.
type CreateUserRequest struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"min=0,max=150"`
}

// UpdateUserRequest is the payload for updating an existing user.
type UpdateUserRequest struct {
	Name string `json:"name,omitempty"`
	Age  int    `json:"age,omitempty"`
}

// ErrorResponse is a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// SuccessResponse is a standard success response.
type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}
