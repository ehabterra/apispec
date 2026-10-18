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

package engine

import (
	"path/filepath"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestInfoCarriesNoToolDefaults pins that the generated info block describes
// the user's API only. The defaults used to stamp the tool author's contact
// (name, URL, email) and an apispec copyright notice into every spec, so a
// user publishing their document advertised someone else as its contact.
func TestInfoCarriesNoToolDefaults(t *testing.T) {
	gen := func(t *testing.T, mutate func(*EngineConfig)) intspec.Info {
		t.Helper()
		cfg := DefaultEngineConfig()
		cfg.InputDir = filepath.Join("..", "..", "testdata", "gin")
		if mutate != nil {
			mutate(cfg)
		}
		out, err := NewEngine(cfg).GenerateOpenAPI()
		if err != nil {
			t.Fatalf("GenerateOpenAPI: %v", err)
		}
		return out.Info
	}

	t.Run("defaults add no contact or description", func(t *testing.T) {
		info := gen(t, nil)
		if info.Contact != nil {
			t.Errorf("contact = %+v, want it omitted", *info.Contact)
		}
		if info.Description != "" {
			t.Errorf("description = %q, want empty", info.Description)
		}
	})

	t.Run("the user's own values are kept verbatim", func(t *testing.T) {
		info := gen(t, func(c *EngineConfig) {
			c.Description = "Orders API."
			c.ContactEmail = "api@example.com"
		})
		if info.Description != "Orders API." {
			t.Errorf("description = %q, want the supplied one with nothing appended", info.Description)
		}
		want := intspec.Contact{Email: "api@example.com"}
		if info.Contact == nil || *info.Contact != want {
			t.Errorf("contact = %+v, want %+v", info.Contact, want)
		}
	})
}
