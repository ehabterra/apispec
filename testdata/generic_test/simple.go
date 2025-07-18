package main

import (
	"encoding/json"
	"net/http"
)

// Simple generic function for testing
func DecodeJSONSimple[T any, TResult any](r *http.Request) (T, TResult, error) {
	var data T
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&data)
	return data, *new(TResult), err
}

// Test struct
type TestRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func testGenericExtraction() {
	r := &http.Request{}

	// This should trigger generic type parameter extraction
	request, _, err := DecodeJSONSimple[TestRequest, TestRequest](r)
	_ = request
	_ = err

	// // Test with different type
	// response, err := DecodeJSONSimple[string](r)
	// _ = response
	// _ = err
}
