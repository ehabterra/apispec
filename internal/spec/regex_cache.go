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
