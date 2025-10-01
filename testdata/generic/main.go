package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/exp/constraints"
	"golang.org/x/exp/slices"
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

// Generic slice operations using golang.org/x/exp/slices
// These functions test go/types.TypeOf with generic slices

// Generic slice filter function
func Filter[T any](slice []T, predicate func(T) bool) []T {
	return slices.DeleteFunc(slice, func(t T) bool { return !predicate(t) })
}

// Generic slice map function
func Map[T, U any](slice []T, mapper func(T) U) []U {
	result := make([]U, len(slice))
	for i, v := range slice {
		result[i] = mapper(v)
	}
	return result
}

// Generic slice reduce function
func Reduce[T, U any](slice []T, initial U, reducer func(U, T) U) U {
	result := initial
	for _, v := range slice {
		result = reducer(result, v)
	}
	return result
}

// Generic slice contains function
func Contains[T comparable](slice []T, item T) bool {
	return slices.Contains(slice, item)
}

// Generic slice sort function
func Sort[T constraints.Ordered](slice []T) {
	slices.Sort(slice)
}

// Generic slice sort with custom comparator
func SortBy[T any](slice []T, less func(T, T) bool) {
	slices.SortFunc(slice, func(a, b T) int {
		if less(a, b) {
			return -1
		}
		if less(b, a) {
			return 1
		}
		return 0
	})
}

// Generic slice unique function
func Unique[T comparable](slice []T) []T {
	return slices.Compact(slice)
}

// Generic slice binary search
func BinarySearch[T constraints.Ordered](slice []T, target T) (int, bool) {
	return slices.BinarySearch(slice, target)
}

// Generic slice operations on user data
func processUsers() {
	// Test various slice operations with generics

	// Filter active users
	activeUsers := Filter(users, func(user GetUserResponse) bool {
		return user.IsActive
	})

	// Map users to names
	userNames := Map(users, func(user GetUserResponse) string {
		return user.Name
	})

	// Count total age
	totalAge := Reduce(users, 0, func(sum int, user GetUserResponse) int {
		return sum + user.Age
	})

	// Check if specific user exists
	hasAlice := Contains(userNames, "Alice")

	// Sort users by age
	usersByAge := make([]GetUserResponse, len(users))
	copy(usersByAge, users)
	SortBy(usersByAge, func(a, b GetUserResponse) bool {
		return a.Age < b.Age
	})

	// Get unique ages
	ages := Map(users, func(user GetUserResponse) int { return user.Age })
	uniqueAges := Unique(ages)
	Sort(uniqueAges)

	// Binary search for specific age
	if len(uniqueAges) > 0 {
		_, found := BinarySearch(uniqueAges, 30)
		_ = found // Use found to avoid unused variable warning
	}

	// Use the processed data
	_ = activeUsers
	_ = userNames
	_ = totalAge
	_ = hasAlice
	_ = usersByAge
	_ = uniqueAges
}

// Generic API response builder with slice operations
func buildPaginatedResponse[T any](items []T, page, limit int) APIResponse[map[string]interface{}] {
	// Sort items for consistent pagination
	SortBy(items, func(a, b T) bool {
		// This is a placeholder - in real code you'd have a proper comparison
		return true
	})

	// Calculate pagination
	start := (page - 1) * limit
	end := start + limit

	if start >= len(items) {
		return APIResponse[map[string]interface{}]{
			Success: true,
			Data: map[string]interface{}{
				"items": []T{},
				"total": len(items),
				"page":  page,
				"limit": limit,
			},
		}
	}

	if end > len(items) {
		end = len(items)
	}

	paginatedItems := items[start:end]

	return APIResponse[map[string]interface{}]{
		Success: true,
		Data: map[string]interface{}{
			"items": paginatedItems,
			"total": len(items),
			"page":  page,
			"limit": limit,
		},
	}
}

// Generic search function using slices
func searchUsers(query string) []GetUserResponse {
	return Filter(users, func(user GetUserResponse) bool {
		return Contains([]string{user.Name, user.Email}, query) ||
			user.Name == query || user.Email == query
	})
}

// Generic statistics function
func calculateUserStats() map[string]interface{} {
	if len(users) == 0 {
		return map[string]interface{}{
			"count": 0,
		}
	}

	ages := Map(users, func(user GetUserResponse) int { return user.Age })
	Sort(ages)

	// Calculate statistics using slice operations
	minAge := ages[0]
	maxAge := ages[len(ages)-1]

	totalAge := Reduce(ages, 0, func(sum, age int) int { return sum + age })
	avgAge := float64(totalAge) / float64(len(ages))

	// Count active users
	activeCount := Reduce(users, 0, func(count int, user GetUserResponse) int {
		if user.IsActive {
			return count + 1
		}
		return count
	})

	return map[string]interface{}{
		"count":     len(users),
		"active":    activeCount,
		"min_age":   minAge,
		"max_age":   maxAge,
		"avg_age":   avgAge,
		"total_age": totalAge,
	}
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
	// Test generic type parameter extraction with slices
	testGenericSliceOperations()

	// Set up routes with generic handlers
	http.HandleFunc("/api/email/send", HandleRequest(handleSendEmail))
	http.HandleFunc("/api/users", HandleRequest(handleCreateUser))
	http.HandleFunc("/api/users/list", HandleRequest(handleListUsers))

	// Start server
	println("Starting server on :8080")
	http.ListenAndServe(":8080", nil)
}

// Test function to demonstrate generic slice operations
func testGenericSliceOperations() {
	println("Testing generic slice operations with golang.org/x/exp/slices...")

	// Test with different types
	testIntSlices()
	testStringSlices()
	testUserSlices()

	// Test complex generic operations
	processUsers()

	// Test API response building
	testPaginatedResponse()

	// Test search functionality
	testSearch()

	// Test statistics
	testStatistics()
}

func testIntSlices() {
	numbers := []int{3, 1, 4, 1, 5, 9, 2, 6}

	// Test sorting
	Sort(numbers)
	println("Sorted numbers:", numbers)

	// Test binary search
	index, found := BinarySearch(numbers, 5)
	println("Binary search for 5:", index, found)

	// Test unique
	uniqueNumbers := Unique(numbers)
	println("Unique numbers:", uniqueNumbers)

	// Test contains
	hasThree := Contains(numbers, 3)
	println("Contains 3:", hasThree)
}

func testStringSlices() {
	names := []string{"Alice", "Bob", "Charlie", "Alice", "David"}

	// Test filtering
	longNames := Filter(names, func(name string) bool {
		return len(name) > 4
	})
	println("Long names:", longNames)

	// Test mapping
	nameLengths := Map(names, func(name string) int {
		return len(name)
	})
	println("Name lengths:", nameLengths)

	// Test reducing
	totalLength := Reduce(nameLengths, 0, func(sum, length int) int {
		return sum + length
	})
	println("Total length:", totalLength)
}

func testUserSlices() {
	// Test with user data
	activeUsers := Filter(users, func(user GetUserResponse) bool {
		return user.IsActive
	})
	println("Active users count:", len(activeUsers))

	// Test mapping to names
	userNames := Map(users, func(user GetUserResponse) string {
		return user.Name
	})
	println("User names:", userNames)

	// Test sorting by age
	usersCopy := make([]GetUserResponse, len(users))
	copy(usersCopy, users)
	SortBy(usersCopy, func(a, b GetUserResponse) bool {
		return a.Age < b.Age
	})
	println("Users sorted by age:", usersCopy[0].Name, usersCopy[1].Name)
}

func testPaginatedResponse() {
	// Test paginated response with different types
	intResponse := buildPaginatedResponse([]int{1, 2, 3, 4, 5}, 1, 3)
	println("Int paginated response:", intResponse.Success)

	stringResponse := buildPaginatedResponse([]string{"a", "b", "c", "d", "e"}, 2, 2)
	println("String paginated response:", stringResponse.Success)

	userResponse := buildPaginatedResponse(users, 1, 10)
	println("User paginated response:", userResponse.Success)
}

func testSearch() {
	// Test search functionality
	results := searchUsers("Alice")
	println("Search results for 'Alice':", len(results))

	results = searchUsers("bob@example.com")
	println("Search results for 'bob@example.com':", len(results))
}

func testStatistics() {
	// Test statistics calculation
	stats := calculateUserStats()
	println("User statistics:", stats)
}
