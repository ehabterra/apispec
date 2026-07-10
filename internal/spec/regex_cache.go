package spec

import (
	"regexp"
	"sync"
)

// Shared compile cache for regexes used across pattern matching, mount
// extraction, parameter parsing, and validator-tag scanning. Compiling
// the same pattern repeatedly was a measurable cost when this project
// first ran on large codebases (pattern matchers iterate over every
// call-graph edge), and the two earlier copies of this cache —
// previously in pattern_matchers.go and mapper.go — drifted in subtle
// ways. Keeping one cache here avoids that drift and lets all callers
// benefit from one shared compile.
var (
	regexCache   = make(map[string]*regexp.Regexp)
	regexCacheMu sync.RWMutex
)

// cachedRegex returns a cached compiled regex, compiling on miss.
// Concurrent callers coexist via a double-checked write lock.
func cachedRegex(pattern string) (*regexp.Regexp, error) {
	regexCacheMu.RLock()
	if re, ok := regexCache[pattern]; ok {
		regexCacheMu.RUnlock()
		return re, nil
	}
	regexCacheMu.RUnlock()

	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()

	// Double-check after acquiring the write lock — another goroutine
	// may have compiled it between our RUnlock and Lock.
	if re, ok := regexCache[pattern]; ok {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache[pattern] = re
	return re, nil
}

// mustCachedRegex is the panic-on-error variant for compile-time-constant
// regex literals — the developer has already validated the pattern, so a
// compile failure is a contract violation, not a runtime condition.
func mustCachedRegex(pattern string) *regexp.Regexp {
	re, err := cachedRegex(pattern)
	if err != nil {
		panic("spec: invalid regex literal: " + err.Error())
	}
	return re
}

// Match-result cache. Pattern matching runs for every (matcher, node) pair
// during route extraction, and the same (pattern, value) pair recurs
// constantly — patterns come from a fixed config and values from the
// metadata string pool, so the key space is small and bounded. Results are
// immutable, making this a pure memo.
var (
	matchCache   = make(map[string]bool)
	matchCacheMu sync.RWMutex
)

// cachedMatch reports whether value matches the (cached) regex pattern,
// memoizing the result per (pattern, value). Invalid patterns report false,
// matching the previous inline behavior.
func cachedMatch(pattern, value string) bool {
	key := pattern + "\x00" + value
	matchCacheMu.RLock()
	if v, ok := matchCache[key]; ok {
		matchCacheMu.RUnlock()
		return v
	}
	matchCacheMu.RUnlock()

	re, err := cachedRegex(pattern)
	matched := err == nil && re.MatchString(value)

	matchCacheMu.Lock()
	matchCache[key] = matched
	matchCacheMu.Unlock()
	return matched
}
