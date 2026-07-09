package spike_test

// Temporary: diff eager vs lazy tracker output on real-world projects.
// Run with APISPEC_REALWORLD=1. Delete when the lazy tree reaches parity on
// these codebases.

import (
	"os"
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

func TestRealWorldParity(t *testing.T) {
	if os.Getenv("APISPEC_REALWORLD") == "" {
		t.Skip("set APISPEC_REALWORLD=1 to run against local private projects")
	}
	dirs := []string{
		"/Users/ehab/Documents/Work/Yassir/lmd-core",
		"/Users/ehab/Documents/Work/Private/enigma/services/api",
	}
	for _, dir := range dirs {
		paths := map[string]map[string]bool{} // mode -> path set
		for _, legacy := range []bool{true, false} {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = dir
			cfg.UseLazyTracker = !legacy
			spec, err := engine.NewEngine(cfg).GenerateOpenAPI()
			if err != nil {
				t.Fatalf("%s legacy=%v: %v", dir, legacy, err)
			}
			mode := "lazy"
			if legacy {
				mode = "eager"
			}
			set := map[string]bool{}
			for p := range spec.Paths {
				set[p] = true
			}
			paths[mode] = set
			t.Logf("%s %s: %d paths", dir, mode, len(set))
		}
		var missing, extra []string
		for p := range paths["eager"] {
			if !paths["lazy"][p] {
				missing = append(missing, p)
			}
		}
		for p := range paths["lazy"] {
			if !paths["eager"][p] {
				extra = append(extra, p)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		for _, p := range missing {
			t.Logf("MISSING %s", p)
		}
		for _, p := range extra {
			t.Logf("EXTRA   %s", p)
		}
	}
}
