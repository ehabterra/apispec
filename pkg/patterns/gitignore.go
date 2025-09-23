// Package patterns provides gitignore-style pattern matching functionality
// that can be used across the project for filtering files, packages, and other paths.
package patterns

import (
	"regexp"
	"strings"
)

// Match checks if a path matches a gitignore-style pattern.
// Supports: *, **, ?, [...], leading/trailing slashes, and negation with !
//
// Examples:
//   - "*.go" matches "main.go" but not "main.txt"
//   - "**/*.go" matches "src/main.go" and "src/pkg/util.go"
//   - "github.com/org/repo/**" matches all subdirectories
//   - "github.com/org/repo/**/*" matches all files in subdirectories
//   - "!*.tmp" negates the pattern (excludes .tmp files)
//   - "test[0-9].go" matches "test1.go", "test2.go", etc.
func Match(pattern, path string) bool {
	return matchGitignorePattern(pattern, path)
}

// matchGitignorePattern implements gitignore-style pattern matching
func matchGitignorePattern(pattern, path string) bool {
	// Handle negation patterns (starting with !)
	negated := false
	if strings.HasPrefix(pattern, "!") {
		negated = true
		pattern = pattern[1:]
	}

	// Empty pattern matches nothing
	if pattern == "" {
		return false
	}

	// Normalize paths - remove leading slashes for consistent matching
	pattern = strings.TrimPrefix(pattern, "/")
	path = strings.TrimPrefix(path, "/")

	// Convert pattern to regex
	regex := patternToRegex(pattern)

	// Compile and match
	matched, err := regexp.MatchString(regex, path)
	if err != nil {
		// Fall back to simple string matching on regex error
		matched = pattern == path
	}

	// Apply negation if needed
	if negated {
		return !matched
	}

	return matched
}

// patternToRegex converts a gitignore pattern to a regex
func patternToRegex(pattern string) string {
	// Handle special case: if pattern ends with /, we need special logic
	trailingSlash := strings.HasSuffix(pattern, "/")
	if trailingSlash {
		// Remove the trailing slash for processing, we'll handle it at the end
		pattern = pattern[:len(pattern)-1]
	}

	// Check if pattern contains any slashes (excluding leading slash)
	hasSlash := strings.Contains(strings.TrimPrefix(pattern, "/"), "/")

	// Start building regex
	var result strings.Builder
	result.WriteString("^")

	// If pattern has no slashes, it should match the filename anywhere in the path
	if !hasSlash && !strings.HasPrefix(pattern, "/") {
		result.WriteString("(?:.*/)?")
	}

	i := 0
	for i < len(pattern) {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// Handle ** - matches any number of path segments
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					// **/ - matches zero or more path segments followed by /
					result.WriteString("(?:.*?/)?")
					i += 3
				} else if i+2 == len(pattern) {
					// ** at end - matches the current path and any subpaths
					// Check if there's a preceding slash that we need to make optional
					if i > 0 && pattern[i-1] == '/' {
						// Remove the last slash from result and make the whole /...** part optional
						resultStr := result.String()
						if strings.HasSuffix(resultStr, "/") {
							result.Reset()
							result.WriteString(resultStr[:len(resultStr)-1])
							result.WriteString("(?:/.*)?")
						} else {
							result.WriteString("(?:/.*)?")
						}
					} else {
						// No preceding slash, just match any characters
						result.WriteString(".*")
					}
					i += 2
				} else {
					// ** in middle - matches zero or more path segments
					result.WriteString(".*")
					i += 2
				}
			} else {
				// Single * - matches anything except /
				result.WriteString("[^/]*")
				i++
			}
		case '?':
			// ? matches any single character except /
			result.WriteString("[^/]")
			i++
		case '[':
			// Character class - find the closing ]
			j := i + 1
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j < len(pattern) {
				// Valid character class
				charClass := pattern[i : j+1]
				// Escape any regex special chars inside the class
				charClass = strings.ReplaceAll(charClass, "\\", "\\\\")
				result.WriteString(charClass)
				i = j + 1
			} else {
				// No closing ], treat as literal [
				result.WriteString("\\[")
				i++
			}
		case '/':
			// Literal slash
			result.WriteString("/")
			i++
		case '.', '^', '$', '(', ')', '{', '}', '+', '|', '\\':
			// Escape regex special characters
			result.WriteString("\\")
			result.WriteByte(pattern[i])
			i++
		default:
			// Regular character
			result.WriteByte(pattern[i])
			i++
		}
	}

	// Handle trailing slash - if original pattern ended with /, match directory and its contents
	if trailingSlash {
		result.WriteString("(?:|/.*)")
	}

	result.WriteString("$")
	return result.String()
}

// MatchAny checks if a path matches any of the given patterns.
// Returns true if any pattern matches, false otherwise.
func MatchAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if Match(pattern, path) {
			return true
		}
	}
	return false
}

// Filter filters a list of paths based on include and exclude patterns.
// Include patterns are applied first, then exclude patterns are applied to remove matches.
// If no include patterns are provided, all paths are initially included.
func Filter(paths []string, includePatterns, excludePatterns []string) []string {
	var result []string

	for _, path := range paths {
		// Check include patterns first
		included := len(includePatterns) == 0 // If no include patterns, include by default
		if !included {
			included = MatchAny(includePatterns, path)
		}

		// If included, check exclude patterns
		if included {
			excluded := MatchAny(excludePatterns, path)
			if !excluded {
				result = append(result, path)
			}
		}
	}

	return result
}
