package models

// User is the request/response payload for the user endpoints.
type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}
