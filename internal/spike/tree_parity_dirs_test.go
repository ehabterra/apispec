package spike_test

// Tree-parity meter for arbitrary local codebases: runs the full engine with
// the eager and the lazy tracker tree over each directory in
// APISPEC_PARITY_DIRS (colon-separated) and reports path-set differences.
// This is the acceptance meter for making the lazy tree the default: it must
// read zero MISSING on every wiring style the legacy tree supports.

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

func TestTreeParityDirs(t *testing.T) {
	dirsEnv := os.Getenv("APISPEC_PARITY_DIRS")
	if dirsEnv == "" {
		t.Skip("set APISPEC_PARITY_DIRS=/path/one:/path/two to diff both trees over local codebases")
	}
	for _, dir := range strings.Split(dirsEnv, ":") {
		if dir == "" {
			continue
		}
		paths := map[string]map[string]string{} // mode -> path -> marshaled item
		for _, mode := range []string{"eager", "lazy"} {
			cfg := engine.DefaultEngineConfig()
			cfg.InputDir = dir
			cfg.UseLazyTracker = mode == "lazy"
			spec, err := engine.NewEngine(cfg).GenerateOpenAPI()
			if err != nil {
				t.Fatalf("%s %s: %v", dir, mode, err)
			}
			items := map[string]string{}
			for p, item := range spec.Paths {
				b, _ := json.Marshal(item)
				items[p] = string(b)
			}
			paths[mode] = items
			t.Logf("%s %s: %d paths", dir, mode, len(items))
		}
		var missing, extra, differing []string
		for p, eagerItem := range paths["eager"] {
			lazyItem, ok := paths["lazy"][p]
			switch {
			case !ok:
				missing = append(missing, p)
			case eagerItem != lazyItem:
				differing = append(differing, p)
			}
		}
		for p := range paths["lazy"] {
			if _, ok := paths["eager"][p]; !ok {
				extra = append(extra, p)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)
		sort.Strings(differing)
		for _, p := range missing {
			t.Logf("MISSING %s", p)
		}
		for _, p := range extra {
			t.Logf("EXTRA   %s", p)
		}
		for _, p := range differing {
			t.Logf("CONTENT-DIFF %s", p)
		}
		t.Logf("%s: %d missing, %d extra, %d content-diff (of %d eager paths)",
			dir, len(missing), len(extra), len(differing), len(paths["eager"]))
	}
}
