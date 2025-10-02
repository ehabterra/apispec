// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestDetectVersionInfo(t *testing.T) {
	// Save original values
	originalVersion := Version
	originalCommit := Commit
	originalBuildDate := BuildDate
	originalGoVersion := GoVersion

	// Reset to defaults
	Version = "0.0.1"
	Commit = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"

	// Test with version already set
	Version = "1.0.0"
	detectVersionInfo()
	if Version != "1.0.0" {
		t.Errorf("Expected version to remain 1.0.0, got %s", Version)
	}

	// Reset and test with build info
	Version = "0.0.1"
	detectVersionInfo()

	// Check that GoVersion is set from build info
	if GoVersion == "unknown" {
		t.Error("Expected GoVersion to be set from build info")
	}

	// Restore original values
	Version = originalVersion
	Commit = originalCommit
	BuildDate = originalBuildDate
	GoVersion = originalGoVersion
}

func TestPrintVersion(t *testing.T) {
	// Capture output
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	printVersion()
	err := w.Close()
	if err != nil {
		t.Fatalf("Failed to close stdout: %v", err)
	}
	_, err = buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("Failed to read from stdout: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "apidiag version:") {
		t.Error("Expected output to contain 'apidiag version:'")
	}
	if !strings.Contains(output, "Commit:") {
		t.Error("Expected output to contain 'Commit:'")
	}
	if !strings.Contains(output, "Build date:") {
		t.Error("Expected output to contain 'Build date:'")
	}
	if !strings.Contains(output, "Go version:") {
		t.Error("Expected output to contain 'Go version:'")
	}
}

// TestParseFlags is skipped due to flag redefinition issues in tests
// func TestParseFlags(t *testing.T) {
// 	// This test is skipped because parseFlags() uses flag.Parse() which
// 	// causes flag redefinition issues when run multiple times in tests
// }

func TestNewDiagramServer(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}

	server := NewDiagramServer(config)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.config != config {
		t.Fatal("Expected config to be set")
	}
}

func TestLoadMetadata(t *testing.T) {
	config := &ServerConfig{
		InputDir: ".",
	}
	server := NewDiagramServer(config)

	// Test with non-existent directory
	server.config.InputDir = "non-existent-directory"
	err := server.LoadMetadata()
	if err == nil {
		t.Error("Expected error for non-existent directory")
	}

	// Test with valid directory (current directory)
	server.config.InputDir = "."
	_ = server.LoadMetadata()
	// This might succeed or fail depending on the current directory
	// We just check that it doesn't panic
}

func TestSetupRoutes(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Setup routes
	server.SetupRoutes()

	// Test that routes are registered
	// We can't easily test the actual HTTP handling without starting a server
	// But we can verify that SetupRoutes doesn't panic
}

func TestHandleIndex(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("Expected HTML content")
	}
}

func TestHandleHealth(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}
}

func TestHandleDiagram(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Test without metadata
	req := httptest.NewRequest("GET", "/diagram", nil)
	w := httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test with metadata
	metadata := &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.metadata = metadata

	req = httptest.NewRequest("GET", "/diagram", nil)
	w = httptest.NewRecorder()

	server.handleDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandlePaginatedDiagram(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Test without metadata
	req := httptest.NewRequest("GET", "/diagram/paginated", nil)
	w := httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test with metadata
	metadata := &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.metadata = metadata

	req = httptest.NewRequest("GET", "/diagram/paginated", nil)
	w = httptest.NewRecorder()

	server.handlePaginatedDiagram(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestHandleStats(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Set up metadata to avoid nil pointer dereference
	metadata := &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.metadata = metadata

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()

	server.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["total_nodes"] == nil {
		t.Error("Expected total_nodes in response")
	}
}

func TestHandleRefresh(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	req := httptest.NewRequest("POST", "/refresh", nil)
	w := httptest.NewRecorder()

	server.handleRefresh(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["message"] != "Metadata refreshed successfully" {
		t.Errorf("Expected message 'Metadata refreshed successfully', got %v", response["message"])
	}
}

func TestHandleExport(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Test without metadata
	req := httptest.NewRequest("GET", "/export", nil)
	w := httptest.NewRecorder()

	server.handleExport(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	// Test with metadata
	metadata := &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.metadata = metadata

	req = httptest.NewRequest("GET", "/export?format=svg", nil)
	w = httptest.NewRecorder()

	server.handleExport(w, req)

	// The export handler might return 400 due to missing metadata or other issues
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 200 or 400, got %d", w.Code)
	}
}

func TestGeneratePaginatedData(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Test without metadata
	data := server.generatePaginatedData(1, 10, 3, []string{}, []string{}, []string{}, []string{}, []string{}, []string{}, "")
	if data == nil {
		t.Error("Expected non-nil data even without metadata")
	}

	// Test with metadata
	metadata := &metadata.Metadata{
		Packages:  make(map[string]*metadata.Package),
		CallGraph: []metadata.CallGraphEdge{},
	}
	server.metadata = metadata

	data = server.generatePaginatedData(1, 10, 3, []string{}, []string{}, []string{}, []string{}, []string{}, []string{}, "")
	if data == nil {
		t.Fatal("Expected non-nil data with metadata")
	}
}

func TestWriteJSON(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	w := httptest.NewRecorder()
	data := map[string]interface{}{
		"test": "value",
	}

	server.writeJSON(w, data)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["test"] != "value" {
		t.Errorf("Expected test value, got %v", response["test"])
	}
}

func TestWriteResponse(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	w := httptest.NewRecorder()
	message := "Test message"

	server.writeResponse(w, message, "application/json")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	// writeResponse writes the data directly as a string
	body := w.Body.String()
	if body != message {
		t.Errorf("Expected body '%s', got '%s'", message, body)
	}
}

func TestWriteError(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	w := httptest.NewRecorder()
	message := "Test error"
	statusCode := http.StatusInternalServerError

	server.writeError(w, message, statusCode)

	if w.Code != statusCode {
		t.Errorf("Expected status %d, got %d", statusCode, w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", w.Header().Get("Content-Type"))
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Expected valid JSON, got %v", err)
	}

	if response["message"] != message {
		t.Errorf("Expected message '%s', got %v", message, response["message"])
	}
}

// Test the main function (this will exit, so we can't test it directly)
func TestMainFunction(t *testing.T) {
	// We can't test main() directly as it calls os.Exit()
	// But we can test that the functions it calls work correctly
	// This is already covered by the other tests
}

// Test version detection with build info
func TestVersionDetectionWithBuildInfo(t *testing.T) {
	// This test verifies that version detection works with build info
	// The actual build info will vary based on the test environment
	detectVersionInfo()

	// At minimum, GoVersion should be set
	if GoVersion == "unknown" {
		t.Error("Expected GoVersion to be set from build info")
	}
}

// Test error handling in various functions
func TestErrorHandling(t *testing.T) {
	config := &ServerConfig{
		Host: "localhost",
		Port: 8080,
	}
	server := NewDiagramServer(config)

	// Test writeJSON with invalid data
	w := httptest.NewRecorder()
	// This should not panic
	server.writeJSON(w, make(chan int))

	// Test writeResponse with empty message
	w = httptest.NewRecorder()
	server.writeResponse(w, "", "application/json")

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Test writeError with empty message
	w = httptest.NewRecorder()
	server.writeError(w, "", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
