package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Generic decoder function - this is what we're testing
func DecodeJSON[TData any](r *http.Request, v interface{}) (TData, error) {
	var data TData
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&data)
	if err != nil {
		return data, err
	}
	return data, nil
}

// Generic response wrapper
type APIResponse[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    T      `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Generic handler function
func HandleRequest[TRequest any, TResponse any](
	handler func(TRequest) (TResponse, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request TRequest
		request, err := DecodeJSON[TRequest](r, nil)
		if err != nil {
			respondWithError(w, "Invalid request", http.StatusBadRequest)
			return
		}

		response, err := handler(request)
		if err != nil {
			respondWithError(w, err.Error(), http.StatusInternalServerError)
			return
		}

		respondWithSuccess(w, response)
	}
}

// Test structs
type SendEmailRequest struct {
	To      string `json:"to" binding:"required"`
	Subject string `json:"subject" binding:"required"`
	Body    string `json:"body" binding:"required"`
}

type SendEmailResponse struct {
	MessageID string    `json:"message_id"`
	SentAt    time.Time `json:"sent_at"`
	Status    string    `json:"status"`
}

type CreateUserRequest struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Age      int    `json:"age"`
	IsActive bool   `json:"is_active"`
}

type CreateUserResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type GetUserResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Age       int       `json:"age"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type ListUsersResponse struct {
	Users []GetUserResponse `json:"users"`
	Total int               `json:"total"`
	Page  int               `json:"page"`
	Limit int               `json:"limit"`
}

// Mock data store
var users = []GetUserResponse{
	{ID: 1, Name: "Alice", Email: "alice@example.com", Age: 30, IsActive: true, CreatedAt: time.Now()},
	{ID: 2, Name: "Bob", Email: "bob@example.com", Age: 25, IsActive: true, CreatedAt: time.Now()},
}

// Handler implementations
func handleSendEmail(req SendEmailRequest) (SendEmailResponse, error) {
	// Simulate email sending
	return SendEmailResponse{
		MessageID: "msg_" + strconv.FormatInt(time.Now().Unix(), 10),
		SentAt:    time.Now(),
		Status:    "sent",
	}, nil
}

func handleCreateUser(req CreateUserRequest) (CreateUserResponse, error) {
	// Simulate user creation
	newUser := CreateUserResponse{
		ID:        len(users) + 1,
		Name:      req.Name,
		Email:     req.Email,
		CreatedAt: time.Now(),
	}
	users = append(users, GetUserResponse{
		ID:        newUser.ID,
		Name:      newUser.Name,
		Email:     newUser.Email,
		Age:       req.Age,
		IsActive:  req.IsActive,
		CreatedAt: newUser.CreatedAt,
	})
	return newUser, nil
}

func handleListUsers(req struct{}) (ListUsersResponse, error) {
	// Simulate listing users
	return ListUsersResponse{
		Users: users,
		Total: len(users),
		Page:  1,
		Limit: 10,
	}, nil
}

// Helper functions
func respondWithSuccess(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse[interface{}]{
		Success: true,
		Data:    data,
	})
}

func respondWithError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(APIResponse[interface{}]{
		Success: false,
		Error:   message,
	})
}

func main() {
	// Test generic type parameter extraction
	testGenericExtraction()

	// Set up routes with generic handlers
	http.HandleFunc("/api/email/send", HandleRequest(handleSendEmail))
	http.HandleFunc("/api/users", HandleRequest(handleCreateUser))
	http.HandleFunc("/api/users/list", HandleRequest(handleListUsers))

	// Start server
	println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)
}
