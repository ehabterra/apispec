package spec

import (
	"runtime"
	"strings"
	"testing"
)

// RecoverFromPanic is a helper function that recovers from panics and provides
// better error reporting, especially for stack overflow scenarios.
// This function should be used in defer statements to catch panics in tests
// and make them fail gracefully instead of crashing the test runner.
//
// Usage:
//
//	defer RecoverFromPanic(t, "TestName")
func RecoverFromPanic(t *testing.T, testName string) {
	if r := recover(); r != nil {
		// Check if it's a stack overflow
		if err, ok := r.(runtime.Error); ok && strings.Contains(err.Error(), "stack overflow") {
			t.Errorf("Test %s failed with stack overflow: %v", testName, err)
		} else {
			t.Errorf("Test %s panicked: %v", testName, r)
		}

		// Print stack trace for debugging
		buf := make([]byte, 1024)
		n := runtime.Stack(buf, false)
		t.Logf("Stack trace:\n%s", string(buf[:n]))
	}
}

// RunWithPanicRecovery runs a test function with panic recovery.
// This is useful for tests that might panic due to stack overflow or other issues.
//
// Usage:
//
//	RunWithPanicRecovery(t, "TestName", func() {
//	    // test code that might panic
//	})
func RunWithPanicRecovery(t *testing.T, testName string, testFunc func()) {
	defer RecoverFromPanic(t, testName)
	testFunc()
}
