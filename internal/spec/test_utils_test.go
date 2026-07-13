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
