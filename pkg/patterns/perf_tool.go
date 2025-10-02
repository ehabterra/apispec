package patterns

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// PerfResult contains performance measurement results
type PerfResult struct {
	Pattern     string
	Path        string
	Iterations  int
	TotalTime   time.Duration
	AvgTime     time.Duration
	AllocsStart uint64
	AllocsEnd   uint64
	TotalAllocs uint64
}

// String returns a formatted string representation of the performance result
func (r PerfResult) String() string {
	return fmt.Sprintf(
		"Pattern: %-30s | Path: %-40s | Avg: %8.2fµs | Allocs: %d",
		r.Pattern, r.Path, float64(r.AvgTime.Nanoseconds())/1000.0, r.TotalAllocs,
	)
}

// BenchmarkPattern measures the performance of a single pattern against a path
func BenchmarkPattern(pattern, path string, iterations int) PerfResult {
	if iterations <= 0 {
		iterations = 10000
	}

	// Warm up
	for i := 0; i < 100; i++ {
		Match(pattern, path)
	}

	// Force garbage collection before measurement
	runtime.GC()

	// Get initial memory stats
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Measure performance
	start := time.Now()
	for i := 0; i < iterations; i++ {
		Match(pattern, path)
	}
	totalTime := time.Since(start)

	// Get final memory stats
	runtime.ReadMemStats(&m2)

	return PerfResult{
		Pattern:     pattern,
		Path:        path,
		Iterations:  iterations,
		TotalTime:   totalTime,
		AvgTime:     totalTime / time.Duration(iterations),
		AllocsStart: m1.TotalAlloc,
		AllocsEnd:   m2.TotalAlloc,
		TotalAllocs: m2.TotalAlloc - m1.TotalAlloc,
	}
}

// BenchmarkPatterns measures the performance of multiple patterns
func BenchmarkPatterns(testCases []struct{ Pattern, Path string }, iterations int) []PerfResult {
	results := make([]PerfResult, len(testCases))

	for i, tc := range testCases {
		results[i] = BenchmarkPattern(tc.Pattern, tc.Path, iterations)
	}

	return results
}

// QuickBench provides a quick performance test with common patterns
func QuickBench() {
	fmt.Println("🚀 Pattern Matching Performance Test")
	fmt.Println("=====================================")

	testCases := []struct{ Pattern, Path string }{
		// Simple patterns
		{"*.go", "main.go"},
		{"*.txt", "readme.txt"},

		// Single star patterns
		{"src/*.go", "src/main.go"},
		{"test/*_test.go", "test/user_test.go"},

		// Double star patterns
		{"**/*.go", "src/internal/service.go"},
		{"src/**/*.go", "src/pkg/utils/helper.go"},

		// Complex patterns
		{"github.com/org/**/*.go", "github.com/org/repo/internal/handler.go"},
		{"**/internal/**/*.go", "cmd/app/internal/service/user.go"},

		// Character classes and special chars
		{"test[0-9].go", "test1.go"},
		{"file?.txt", "file1.txt"},

		// Negation
		{"!*.tmp", "file.go"},
		{"!**/test/**", "src/main.go"},

		// Trailing slash
		{"src/", "src/main.go"},
		{"docs/", "docs/readme.md"},

		// Long paths
		{"**/*.go", "github.com/very/long/deeply/nested/path/with/many/segments/file.go"},
	}

	results := BenchmarkPatterns(testCases, 1000)

	fmt.Printf("\n%-30s | %-40s | %-12s | %s\n", "Pattern", "Path", "Avg Time", "Allocs")
	fmt.Println(strings.Repeat("-", 110))

	for _, result := range results {
		fmt.Println(result)
	}

	// Summary statistics
	var totalTime time.Duration
	var totalAllocs uint64
	for _, result := range results {
		totalTime += result.AvgTime
		totalAllocs += result.TotalAllocs
	}

	fmt.Printf("\n📊 Summary:\n")
	fmt.Printf("   Total patterns tested: %d\n", len(results))
	fmt.Printf("   Average time per pattern: %.2fµs\n", float64(totalTime.Nanoseconds())/float64(len(results))/1000.0)
	fmt.Printf("   Average allocations per pattern: %d\n", totalAllocs/uint64(len(results)))
}

// ComparePatterns compares the performance of different patterns against the same path
func ComparePatterns(patterns []string, path string, iterations int) {
	fmt.Printf("🔍 Comparing patterns against path: %s\n", path)
	fmt.Println("=" + strings.Repeat("=", len(path)+35))

	results := make([]PerfResult, len(patterns))
	for i, pattern := range patterns {
		results[i] = BenchmarkPattern(pattern, path, iterations)
	}

	// Sort by performance (fastest first)
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].AvgTime > results[j].AvgTime {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	fmt.Printf("%-30s | %-12s | %-10s | %s\n", "Pattern", "Avg Time", "Allocs", "Relative")
	fmt.Println(strings.Repeat("-", 80))

	if len(results) == 0 {
		fmt.Println("No patterns to compare")
		return
	}

	baseline := results[0].AvgTime
	for _, result := range results {
		relative := float64(result.AvgTime) / float64(baseline)
		fmt.Printf("%-30s | %8.2fµs | %8d | %.2fx\n",
			result.Pattern,
			float64(result.AvgTime.Nanoseconds())/1000.0,
			result.TotalAllocs,
			relative,
		)
	}
}

// ProfilePattern provides detailed profiling information for a pattern
func ProfilePattern(pattern, path string, iterations int) {
	fmt.Printf("🔬 Profiling pattern: %s\n", pattern)
	fmt.Printf("   Against path: %s\n", path)
	fmt.Printf("   Iterations: %d\n\n", iterations)

	// Test if pattern matches
	matches := Match(pattern, path)
	fmt.Printf("✅ Pattern matches: %v\n\n", matches)

	// Benchmark the pattern
	result := BenchmarkPattern(pattern, path, iterations)

	fmt.Printf("⏱️  Performance Results:\n")
	fmt.Printf("   Total time: %v\n", result.TotalTime)
	fmt.Printf("   Average time: %.2fµs\n", float64(result.AvgTime.Nanoseconds())/1000.0)
	fmt.Printf("   Operations per second: %.0f\n", float64(time.Second)/float64(result.AvgTime))
	fmt.Printf("   Total allocations: %d bytes\n", result.TotalAllocs)
	fmt.Printf("   Allocations per operation: %.1f bytes\n", float64(result.TotalAllocs)/float64(iterations))

	// Show the generated regex
	regex := patternToRegex(pattern)
	fmt.Printf("\n🔧 Generated regex: %s\n", regex)

	// Performance rating
	avgNs := result.AvgTime.Nanoseconds()
	var rating string
	switch {
	case avgNs < 5000:
		rating = "⭐⭐⭐⭐⭐ Excellent"
	case avgNs < 10000:
		rating = "⭐⭐⭐⭐ Good"
	case avgNs < 20000:
		rating = "⭐⭐⭐ Fair"
	case avgNs < 50000:
		rating = "⭐⭐ Poor"
	default:
		rating = "⭐ Very Poor"
	}
	fmt.Printf("\n📈 Performance Rating: %s\n", rating)
}
