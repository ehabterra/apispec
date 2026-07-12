package spike_test

// Tree-parity meter for arbitrary local codebases: builds metadata once per
// directory in APISPEC_PARITY_DIRS (colon-separated), maps it to OpenAPI
// with the eager and the lazy tracker tree DIRECTLY (independent of the
// engine's tree selection, which is configured for production use), and
// reports path-set and per-path content differences. This is the acceptance
// meter for lazy-tree work: MISSING entries are wiring styles the lazy tree
// does not cover yet.

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/core"
	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// configForDir mirrors the engine's framework detection + default-config
// selection so the meter maps with the same config the engine would use.
func configForDir(t *testing.T, dir string, meta *metadata.Metadata) *spec.APISpecConfig {
	t.Helper()
	framework, err := core.NewFrameworkDetector().Detect(dir)
	if err != nil {
		t.Fatalf("detect %s: %v", dir, err)
	}
	var cfg *spec.APISpecConfig
	switch framework {
	case "gin":
		cfg = spec.DefaultGinConfig()
	case "chi":
		cfg = spec.DefaultChiConfig()
	case "echo":
		cfg = spec.DefaultEchoConfig()
	case "fiber":
		cfg = spec.DefaultFiberConfig()
	case "mux":
		cfg = spec.DefaultMuxConfig()
	default:
		cfg = spec.DefaultHTTPConfig()
	}
	intspec.ApplySecurityPresets(cfg, meta)
	return cfg
}

func TestTreeParityDirs(t *testing.T) {
	dirsEnv := os.Getenv("APISPEC_PARITY_DIRS")
	if dirsEnv == "" {
		t.Skip("set APISPEC_PARITY_DIRS=/path/one:/path/two to diff both trees over local codebases")
	}
	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    engine.DefaultMaxNodesPerTree,
		MaxChildrenPerNode: engine.DefaultMaxChildrenPerNode,
		MaxArgsPerFunction: engine.DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth: engine.DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:  engine.DefaultMaxRecursionDepth,
	}
	genCfg := intspec.GeneratorConfig{OpenAPIVersion: "3.1.1", Title: "parity", APIVersion: "0.0.0"}

	for _, dir := range strings.Split(dirsEnv, ":") {
		if dir == "" {
			continue
		}
		meta := fixtureMetadata(t, dir)
		apiCfg := configForDir(t, dir, meta)

		paths := map[string]map[string]string{} // mode -> path -> marshaled item
		for _, mode := range []string{"eager", "lazy"} {
			var tree intspec.TrackerTreeInterface
			if mode == "lazy" {
				tree = intspec.NewLazyTree(meta, limits)
			} else {
				tree = intspec.NewTrackerTree(meta, limits, nil)
			}
			s, _, err := intspec.MapMetadataToOpenAPIWithDiagnostics(tree, apiCfg, genCfg)
			if err != nil {
				t.Fatalf("%s %s: %v", dir, mode, err)
			}
			items := map[string]string{}
			for p, item := range s.Paths {
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
