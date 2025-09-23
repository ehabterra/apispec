package patterns

import (
	"fmt"
	"strings"
	"testing"
)

// Benchmark different pattern types
func BenchmarkMatch_Simple(b *testing.B) {
	pattern := "*.go"
	path := "main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_SingleStar(b *testing.B) {
	pattern := "src/*/main.go"
	path := "src/app/main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_DoubleStar(b *testing.B) {
	pattern := "src/**/*.go"
	path := "src/internal/pkg/utils/helper.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_ComplexPattern(b *testing.B) {
	pattern := "github.com/org/repo/**/internal/**/*.go"
	path := "github.com/org/repo/cmd/app/internal/service/handler.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_LongPath(b *testing.B) {
	pattern := "github.com/**/*.go"
	path := "github.com/very/long/deeply/nested/path/with/many/segments/and/subdirectories/file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_CharacterClass(b *testing.B) {
	pattern := "test[0-9][a-z].go"
	path := "test1a.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_Question(b *testing.B) {
	pattern := "test?.go"
	path := "test1.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_Negation(b *testing.B) {
	pattern := "!*.tmp"
	path := "file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_TrailingSlash(b *testing.B) {
	pattern := "src/"
	path := "src/main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

// Benchmark MatchAny with different numbers of patterns
func BenchmarkMatchAny_2Patterns(b *testing.B) {
	patterns := []string{"*.go", "*.txt"}
	path := "main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchAny(patterns, path)
	}
}

func BenchmarkMatchAny_5Patterns(b *testing.B) {
	patterns := []string{"*.go", "*.txt", "*.md", "*.yaml", "*.json"}
	path := "main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchAny(patterns, path)
	}
}

func BenchmarkMatchAny_10Patterns(b *testing.B) {
	patterns := []string{
		"*.go", "*.txt", "*.md", "*.yaml", "*.json",
		"**/*.go", "**/*.txt", "src/**", "test/**", "docs/**",
	}
	path := "src/main.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchAny(patterns, path)
	}
}

func BenchmarkMatchAny_Complex(b *testing.B) {
	patterns := []string{
		"github.com/org1/**",
		"github.com/org2/**",
		"github.com/org3/**",
		"**/*.go",
		"**/test/**",
		"**/internal/**",
		"**/*.proto",
		"**/*.yaml",
		"**/*.json",
		"**/*.md",
	}
	path := "github.com/org2/repo/internal/service.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchAny(patterns, path)
	}
}

// Benchmark Filter with different dataset sizes
func BenchmarkFilter_Small(b *testing.B) {
	paths := []string{
		"main.go", "helper.go", "test.go", "readme.txt", "config.yaml",
	}
	includePatterns := []string{"*.go"}
	excludePatterns := []string{"*test*"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(paths, includePatterns, excludePatterns)
	}
}

func BenchmarkFilter_Medium(b *testing.B) {
	paths := make([]string, 100)
	for i := 0; i < 100; i++ {
		switch i % 3 {
		case 0:
			paths[i] = fmt.Sprintf("src/pkg%d/main.go", i)
		case 1:
			paths[i] = fmt.Sprintf("test/pkg%d/test.go", i)
		default:
			paths[i] = fmt.Sprintf("docs/pkg%d/readme.txt", i)
		}
	}
	includePatterns := []string{"**/*.go"}
	excludePatterns := []string{"**/test/**"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(paths, includePatterns, excludePatterns)
	}
}

func BenchmarkFilter_Large(b *testing.B) {
	paths := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		switch i % 5 {
		case 0:
			paths[i] = fmt.Sprintf("github.com/org/repo/pkg%d/main.go", i)
		case 1:
			paths[i] = fmt.Sprintf("github.com/org/repo/internal/pkg%d/service.go", i)
		case 2:
			paths[i] = fmt.Sprintf("github.com/org/repo/test/pkg%d/test.go", i)
		case 3:
			paths[i] = fmt.Sprintf("github.com/org/repo/docs/pkg%d/readme.txt", i)
		case 4:
			paths[i] = fmt.Sprintf("github.com/org/repo/build/pkg%d/temp.tmp", i)
		}
	}
	includePatterns := []string{"github.com/org/repo/**/*.go"}
	excludePatterns := []string{"**/test/**", "*.tmp"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(paths, includePatterns, excludePatterns)
	}
}

// Benchmark regex compilation vs caching (simulate pattern reuse)
func BenchmarkMatch_SamePattern_Repeated(b *testing.B) {
	pattern := "github.com/org/**/*.go"
	paths := []string{
		"github.com/org/repo1/main.go",
		"github.com/org/repo2/service.go",
		"github.com/org/repo3/handler.go",
		"github.com/org/repo4/util.go",
		"github.com/org/repo5/config.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			Match(pattern, path)
		}
	}
}

// Benchmark different path lengths
func BenchmarkMatch_ShortPath(b *testing.B) {
	pattern := "*.go"
	path := "a.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_MediumPath(b *testing.B) {
	pattern := "**/*.go"
	path := "github.com/org/repo/internal/service.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_VeryLongPath(b *testing.B) {
	pattern := "**/*.go"
	path := strings.Repeat("very/long/path/segment/", 20) + "file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

// Benchmark worst-case scenarios
func BenchmarkMatch_ManyStars(b *testing.B) {
	pattern := "*/*/*/*/*/*.go"
	path := "a/b/c/d/e/file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatch_ManyDoubleStar(b *testing.B) {
	pattern := "**/**/**/*.go"
	path := "a/b/c/d/e/f/g/h/i/j/file.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

// Benchmark pattern compilation only (internal function)
func BenchmarkPatternToRegex_Simple(b *testing.B) {
	pattern := "*.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		patternToRegex(pattern)
	}
}

func BenchmarkPatternToRegex_Complex(b *testing.B) {
	pattern := "github.com/org/**/internal/**/*.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		patternToRegex(pattern)
	}
}

// Benchmark real-world scenarios
func BenchmarkRealWorld_GoProject(b *testing.B) {
	// Simulate filtering a typical Go project
	paths := []string{
		"cmd/app/main.go",
		"internal/service/user.go",
		"internal/service/auth.go",
		"internal/handler/http.go",
		"internal/handler/grpc.go",
		"pkg/utils/string.go",
		"pkg/utils/time.go",
		"test/integration/api_test.go",
		"test/unit/service_test.go",
		"docs/api.md",
		"docs/deployment.md",
		"build/Dockerfile",
		"build/docker-compose.yaml",
		"go.mod",
		"go.sum",
		"README.md",
		".gitignore",
		"Makefile",
	}

	includePatterns := []string{"**/*.go"}
	excludePatterns := []string{"**/test/**"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(paths, includePatterns, excludePatterns)
	}
}

func BenchmarkRealWorld_MonorepoFiltering(b *testing.B) {
	// Simulate filtering packages in a large monorepo
	patterns := []string{
		"github.com/company/monorepo/services/**",
		"github.com/company/monorepo/libs/**",
		"github.com/company/monorepo/tools/**",
	}

	paths := []string{
		"github.com/company/monorepo/services/auth/main.go",
		"github.com/company/monorepo/services/user/handler.go",
		"github.com/company/monorepo/services/payment/service.go",
		"github.com/company/monorepo/libs/database/client.go",
		"github.com/company/monorepo/libs/logging/logger.go",
		"github.com/company/monorepo/libs/metrics/collector.go",
		"github.com/company/monorepo/tools/migration/main.go",
		"github.com/company/monorepo/tools/codegen/generator.go",
		"github.com/company/other/repo/service.go", // Should not match
		"github.com/external/dependency/lib.go",    // Should not match
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, path := range paths {
			MatchAny(patterns, path)
		}
	}
}
