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
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/ehabterra/apispec/internal/spec"
)

// serverDemo demonstrates the diagram server functionality
func serverDemo() {
	fmt.Println("🚀 APISpec Diagram Server Demo")
	fmt.Println("==============================")
	fmt.Println()

	// Check if we're in the right directory
	if _, err := os.Stat("go.mod"); os.IsNotExist(err) {
		fmt.Println("❌ Error: This demo should be run from the project root directory")
		fmt.Println("   Please run: cd /path/to/apispec && go run examples/server_demo.go")
		return
	}

	fmt.Println("📋 This demo will:")
	fmt.Println("   1. Build the diagram server")
	fmt.Println("   2. Start the server on localhost:8080")
	fmt.Println("   3. Generate a server-based HTML client")
	fmt.Println("   4. Show you how to access the visualization")
	fmt.Println()

	// Build the server
	fmt.Println("🔨 Building diagram server...")
	buildCmd := exec.Command("go", "build", "-o", "apispec-server", "./cmd/server")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		log.Fatalf("Failed to build server: %v", err)
	}
	fmt.Println("✅ Server built successfully")
	fmt.Println()

	// Generate server-based HTML client
	fmt.Println("📄 Generating server-based HTML client...")
	err := spec.GenerateServerBasedCytoscapeHTML("http://localhost:8080", "server-client.html")
	if err != nil {
		log.Printf("Warning: Failed to generate HTML client: %v", err)
	} else {
		fmt.Println("✅ HTML client generated: server-client.html")
	}
	fmt.Println()

	// Start the server in background
	fmt.Println("🚀 Starting diagram server...")
	serverCmd := exec.Command("./apispec-server",
		"--dir", ".",
		"--port", "8080",
		"--page-size", "100",
		"--max-depth", "3",
		"--verbose")

	// Start server in background
	if err := serverCmd.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Println("✅ Server started successfully")
	fmt.Println()

	// Wait a moment for server to start
	time.Sleep(2 * time.Second)

	fmt.Println("🌐 Server is now running!")
	fmt.Println()
	fmt.Println("📊 Available endpoints:")
	fmt.Println("   • http://localhost:8080/ - Web interface")
	fmt.Println("   • http://localhost:8080/api/diagram/page - Paginated API")
	fmt.Println("   • http://localhost:8080/api/diagram/stats - Statistics")
	fmt.Println("   • http://localhost:8080/health - Health check")
	fmt.Println()
	fmt.Println("🎯 To view the call graph:")
	fmt.Println("   1. Open http://localhost:8080 in your browser")
	fmt.Println("   2. Or open server-client.html in your browser")
	fmt.Println("   3. Use the controls to load and explore the call graph")
	fmt.Println()
	fmt.Println("🔧 API Examples:")
	fmt.Println("   curl http://localhost:8080/api/diagram/page?page=1&size=100")
	fmt.Println("   curl http://localhost:8080/api/diagram/stats")
	fmt.Println("   curl http://localhost:8080/health")
	fmt.Println()
	fmt.Println("⏹️  Press Ctrl+C to stop the server")
	fmt.Println()

	// Wait for server to finish
	if err := serverCmd.Wait(); err != nil {
		log.Printf("Server exited with error: %v", err)
	}

	// Cleanup
	fmt.Println("🧹 Cleaning up...")
	os.Remove("apispec-server")
	fmt.Println("✅ Demo completed")
}

func main() {
	serverDemo()
}
