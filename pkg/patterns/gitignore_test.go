package patterns

import (
	"reflect"
	"testing"
)

func TestMatch(t *testing.T) {
	testCases := []struct {
		pattern string
		path    string
		want    bool
		desc    string
	}{
		// Basic ** patterns
		{"github.com/YAtechnologies/lmd-core/modules/**", "github.com/YAtechnologies/lmd-core/modules/payment/http/handlers", true, "** matches nested directories"},
		{"github.com/YAtechnologies/lmd-core/modules/**", "github.com/YAtechnologies/lmd-core/modules/payment", true, "** matches single subdirectory"},
		{"github.com/YAtechnologies/lmd-core/modules/**", "github.com/YAtechnologies/lmd-core/modules", true, "** matches base directory"},
		{"github.com/YAtechnologies/lmd-core/modules/**", "github.com/YAtechnologies/lmd-core/other", false, "** doesn't match different path"},

		// Complex patterns with **/*
		{"github.com/YAtechnologies/lmd-core/modules/**/*", "github.com/YAtechnologies/lmd-core/modules/payment/http/handlers", true, "**/* matches deeply nested"},
		{"github.com/YAtechnologies/lmd-core/modules/**/*", "github.com/YAtechnologies/lmd-core/modules/payment", true, "**/* matches single level"},
		{"github.com/YAtechnologies/lmd-core/modules/**/*", "github.com/YAtechnologies/lmd-core/modules", false, "**/* requires at least one more level"},

		// Suffix patterns
		{"**/handlers", "github.com/YAtechnologies/lmd-core/modules/payment/http/handlers", true, "suffix with ** matches"},
		{"**/handlers", "github.com/YAtechnologies/lmd-core/modules/user/handlers", true, "suffix with ** matches different path"},
		{"**/handlers", "github.com/YAtechnologies/lmd-core/modules/payment/http/services", false, "suffix with ** doesn't match wrong ending"},

		// Single * patterns
		{"*.go", "main.go", true, "single * matches file extension"},
		{"*.go", "main.txt", false, "single * doesn't match wrong extension"},
		{"test*", "test123", true, "single * matches prefix"},
		{"test*", "other123", false, "single * doesn't match wrong prefix"},

		// ? patterns
		{"test?.go", "test1.go", true, "? matches single character"},
		{"test?.go", "test12.go", false, "? doesn't match multiple characters"},
		{"test?.go", "test.go", false, "? requires a character"},

		// Character classes
		{"test[0-9].go", "test1.go", true, "[0-9] matches digit"},
		{"test[0-9].go", "testa.go", false, "[0-9] doesn't match letter"},
		{"test[abc].go", "testa.go", true, "[abc] matches specific chars"},
		{"test[abc].go", "testd.go", false, "[abc] doesn't match other chars"},

		// Directory patterns
		{"modules/", "modules/payment", true, "trailing / matches directory contents"},
		{"modules/", "modules", true, "trailing / matches directory itself"},
		{"modules/", "other", false, "trailing / doesn't match other paths"},

		// Negation patterns
		{"!*.tmp", "file.txt", true, "negation pattern excludes non-matching"},
		{"!*.tmp", "file.tmp", false, "negation pattern excludes matching"},

		// Complex real-world patterns
		{"src/**/*.go", "src/main.go", true, "src/**/*.go matches direct file"},
		{"src/**/*.go", "src/pkg/utils/helper.go", true, "src/**/*.go matches nested file"},
		{"src/**/*.go", "src/pkg/file.txt", false, "src/**/*.go doesn't match non-go files"},
		{"**/test/**", "internal/test/unit/helper.go", true, "**/test/** matches test directories"},
		{"**/test/**", "internal/pkg/helper.go", false, "**/test/** doesn't match non-test paths"},

		// Edge cases
		{"", "anything", false, "empty pattern matches nothing"},
		{"exact", "exact", true, "exact match works"},
		{"exact", "different", false, "exact non-match works"},
		{"/leading/slash", "leading/slash", true, "leading slash is normalized"},
		{"trailing/slash/", "trailing/slash/file", true, "trailing slash matches contents"},

		// Special characters that need escaping
		{"file.txt", "file.txt", true, "literal dot matches"},
		{"file.txt", "fileXtxt", false, "literal dot doesn't match other chars"},
		{"test(1).go", "test(1).go", true, "parentheses are escaped"},
		{"test+file.go", "test+file.go", true, "plus is escaped"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := Match(tc.pattern, tc.path)
			if got != tc.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
				// Show the regex for debugging
				regex := patternToRegex(tc.pattern)
				t.Logf("Generated regex: %s", regex)
			}
		})
	}
}

func TestMatchAny(t *testing.T) {
	testCases := []struct {
		patterns []string
		path     string
		want     bool
		desc     string
	}{
		{[]string{"*.go", "*.txt"}, "main.go", true, "matches first pattern"},
		{[]string{"*.go", "*.txt"}, "readme.txt", true, "matches second pattern"},
		{[]string{"*.go", "*.txt"}, "image.png", false, "matches no pattern"},
		{[]string{}, "anything", false, "empty patterns match nothing"},
		{[]string{"**/test/**", "**/*.tmp"}, "internal/test/unit.go", true, "matches complex pattern"},
		{[]string{"github.com/org/**", "bitbucket.org/org/**"}, "github.com/org/repo/file.go", true, "matches first org pattern"},
		{[]string{"github.com/org/**", "bitbucket.org/org/**"}, "bitbucket.org/org/repo/file.go", true, "matches second org pattern"},
		{[]string{"github.com/org/**", "bitbucket.org/org/**"}, "gitlab.com/org/repo/file.go", false, "matches no org pattern"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := MatchAny(tc.patterns, tc.path)
			if got != tc.want {
				t.Errorf("MatchAny(%v, %q) = %v, want %v", tc.patterns, tc.path, got, tc.want)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	paths := []string{
		"github.com/org/repo/main.go",
		"github.com/org/repo/internal/helper.go",
		"github.com/org/repo/internal/test/unit.go",
		"github.com/org/repo/pkg/utils/string.go",
		"github.com/org/repo/cmd/app/main.go",
		"github.com/org/repo/docs/readme.txt",
		"github.com/org/repo/build/temp.tmp",
		"github.com/other/repo/main.go",
	}

	testCases := []struct {
		includePatterns []string
		excludePatterns []string
		want            []string
		desc            string
	}{
		{
			includePatterns: []string{"github.com/org/repo/**"},
			excludePatterns: []string{},
			want: []string{
				"github.com/org/repo/main.go",
				"github.com/org/repo/internal/helper.go",
				"github.com/org/repo/internal/test/unit.go",
				"github.com/org/repo/pkg/utils/string.go",
				"github.com/org/repo/cmd/app/main.go",
				"github.com/org/repo/docs/readme.txt",
				"github.com/org/repo/build/temp.tmp",
			},
			desc: "include specific org repo",
		},
		{
			includePatterns: []string{"**/*.go"},
			excludePatterns: []string{},
			want: []string{
				"github.com/org/repo/main.go",
				"github.com/org/repo/internal/helper.go",
				"github.com/org/repo/internal/test/unit.go",
				"github.com/org/repo/pkg/utils/string.go",
				"github.com/org/repo/cmd/app/main.go",
				"github.com/other/repo/main.go",
			},
			desc: "include only Go files",
		},
		{
			includePatterns: []string{},
			excludePatterns: []string{"**/test/**", "*.tmp"},
			want: []string{
				"github.com/org/repo/main.go",
				"github.com/org/repo/internal/helper.go",
				"github.com/org/repo/pkg/utils/string.go",
				"github.com/org/repo/cmd/app/main.go",
				"github.com/org/repo/docs/readme.txt",
				"github.com/other/repo/main.go",
			},
			desc: "exclude test files and temp files",
		},
		{
			includePatterns: []string{"**/*.go"},
			excludePatterns: []string{"**/test/**", "**/cmd/**"},
			want: []string{
				"github.com/org/repo/main.go",
				"github.com/org/repo/internal/helper.go",
				"github.com/org/repo/pkg/utils/string.go",
				"github.com/other/repo/main.go",
			},
			desc: "include Go files but exclude test and cmd directories",
		},
		{
			includePatterns: []string{"github.com/org/**"},
			excludePatterns: []string{"**/internal/**", "*.tmp", "*.txt"},
			want: []string{
				"github.com/org/repo/main.go",
				"github.com/org/repo/pkg/utils/string.go",
				"github.com/org/repo/cmd/app/main.go",
			},
			desc: "complex include and exclude patterns",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := Filter(paths, tc.includePatterns, tc.excludePatterns)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Filter() = %v, want %v", got, tc.want)
				t.Logf("Include patterns: %v", tc.includePatterns)
				t.Logf("Exclude patterns: %v", tc.excludePatterns)
			}
		})
	}
}

func TestPatternToRegex(t *testing.T) {
	testCases := []struct {
		pattern string
		want    string
		desc    string
	}{
		{"*.go", "^(?:.*/)?[^/]*\\.go$", "simple wildcard with escaped dot"},
		{"**/*.go", "^(?:.*?/)?[^/]*\\.go$", "recursive wildcard with file pattern"},
		{"test?", "^(?:.*/)?test[^/]$", "question mark wildcard"},
		{"test[0-9]", "^(?:.*/)?test[0-9]$", "character class"},
		{"dir/", "^(?:.*/)?dir(?:|/.*)$", "directory pattern with trailing slash"},
		{"file.txt", "^(?:.*/)?file\\.txt$", "literal pattern with escaped dot"},
		{"test(1)", "^(?:.*/)?test\\(1\\)$", "escaped parentheses"},
		{"a+b", "^(?:.*/)?a\\+b$", "escaped plus sign"},
		{"a**/b", "^a(?:.*?/)?b$", "** in middle of pattern"},
		{"**/dir/**", "^(?:.*?/)?dir(?:/.*)?$", "** at start and end"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			got := patternToRegex(tc.pattern)
			if got != tc.want {
				t.Errorf("patternToRegex(%q) = %q, want %q", tc.pattern, got, tc.want)
			}
		})
	}
}

// Benchmark tests
func BenchmarkMatch(b *testing.B) {
	pattern := "github.com/org/repo/**/*.go"
	path := "github.com/org/repo/internal/pkg/utils/helper.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Match(pattern, path)
	}
}

func BenchmarkMatchAny(b *testing.B) {
	patterns := []string{
		"github.com/org1/**",
		"github.com/org2/**",
		"github.com/org3/**",
		"**/*.go",
		"**/test/**",
	}
	path := "github.com/org2/repo/internal/service.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MatchAny(patterns, path)
	}
}

func BenchmarkFilter(b *testing.B) {
	paths := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		switch i % 3 {
		case 0:
			paths[i] = "github.com/org/repo/pkg/file" + string(rune(i)) + ".go"
		case 1:
			paths[i] = "github.com/org/repo/test/file" + string(rune(i)) + ".go"
		default:
			paths[i] = "github.com/other/repo/file" + string(rune(i)) + ".txt"
		}
	}

	includePatterns := []string{"**/*.go"}
	excludePatterns := []string{"**/test/**"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Filter(paths, includePatterns, excludePatterns)
	}
}
