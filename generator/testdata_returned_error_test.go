// Copyright 2026 Ehab Terra
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

package generator

import (
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// An error a handler RETURNS documents the status the framework's error
// handler writes for it (issue #556): echo's *HTTPError renders
// `{"message": …}`, fiber's *Error its message as text.
func TestTestdata_ReturnedError(t *testing.T) {
	for _, fw := range []struct {
		name, fixture string
		cfg           *intspec.APISpecConfig
		want          map[string]string // status -> media type ("" = status only)
	}{
		{"echo", "returned_error_echo", intspec.DefaultEchoConfig(), map[string]string{
			"404": "application/json",
			// Returned by middleware in front of the handler.
			"401": "application/json",
			// A returned sentinel: `return echo.ErrForbidden`.
			"403": "application/json",
		}},
		{"fiber", "returned_error_fiber", intspec.DefaultFiberConfig(), map[string]string{
			"404": "text/plain; charset=utf-8",
			"409": "",
			// A returned sentinel: `return fiber.ErrForbidden`.
			"403": "text/plain; charset=utf-8",
		}},
	} {
		t.Run(fw.name, func(t *testing.T) {
			out := loadTestdata(t, fw.fixture, fw.cfg)
			noDanglingRefs(t, out)
			op := opFor(out.Paths["/items/{id}"], "GET")
			if op == nil {
				t.Fatalf("GET /items/{id} missing; have %v", mapPathKeys(out.Paths))
			}
			if _, ok := op.Responses["200"]; !ok {
				t.Errorf("lost its 200; have %v", statusKeys(op))
			}
			for status, media := range fw.want {
				resp, ok := op.Responses[status]
				if !ok {
					t.Errorf("no %s — the status of the error it returns; have %v", status, statusKeys(op))
					continue
				}
				if media == "" {
					continue
				}
				if _, ok := resp.Content[media]; !ok {
					t.Errorf("%s content = %v, want %s — what the framework's error handler writes", status, resp.Content, media)
				}
			}
		})
	}
}
