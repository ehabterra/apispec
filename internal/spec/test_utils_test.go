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
	"testing"
)

// RecoverFromPanic turns a recoverable panic into a test failure with a stack
// trace instead of aborting the whole test binary. Note it cannot catch stack
// exhaustion — Go reports that as a fatal error that recover() never sees;
// runaway recursion must be prevented by explicit depth/cycle bounds.
//
// Usage:
//
//	defer RecoverFromPanic(t, "TestName")
func RecoverFromPanic(t *testing.T, testName string) {
	if r := recover(); r != nil {
		t.Errorf("Test %s panicked: %v", testName, r)

		// Print stack trace for debugging
		buf := make([]byte, 1024)
		n := runtime.Stack(buf, false)
		t.Logf("Stack trace:\n%s", string(buf[:n]))
	}
}

// RunWithPanicRecovery runs a test function, converting a recoverable panic
// into a test failure (see RecoverFromPanic for the stack-exhaustion caveat).
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
