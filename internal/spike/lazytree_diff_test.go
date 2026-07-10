// LazyTree parity harness — docs/TRACKER_REDESIGN.md step 4.
//
// Runs the full metadata→OpenAPI mapping twice per fixture — once with the
// eager TrackerTree, once with the LazyTree — and diffs the outputs.
// Fixtures listed without a knownDiff reason MUST be byte-identical (this is
// the regression guard for LazyTree work); the knownDiff entries document
// the two remaining, understood divergences.
package spike_test

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

func TestLazyTreeParity(t *testing.T) {
	fixtures := []struct {
		dir       string
		cfg       func() *spec.APISpecConfig
		cfgFile   string // optional used-config.yaml, overrides cfg
		knownDiff string // empty = must be byte-identical
	}{
		{"../../testdata/mux_path_params", spec.DefaultMuxConfig, "",
			""},
		{"../../testdata/mux", spec.DefaultMuxConfig, "",
			""},
		{"../../testdata/chi", spec.DefaultChiConfig, "",
			""},
		{"../../testdata/gin", spec.DefaultGinConfig, "",
			""},
		{"../../testdata/echo", spec.DefaultEchoConfig, "",
			""},
		{"../../testdata/echo_handler_factory", spec.DefaultEchoConfig, "",
			""},
		{"../../testdata/fiber", spec.DefaultFiberConfig, "",
			""},
		{"../../testdata/servemux", spec.DefaultHTTPConfig, "",
			""},
		{"../../testdata/wrapped_response", spec.DefaultHTTPConfig, "",
			""},
		{"../../testdata/helper_response_body", spec.DefaultHTTPConfig, "",
			""},
		{"../../testdata/router_mount_options", spec.DefaultChiConfig, "", ""},
		{"../../testdata/generic", spec.DefaultHTTPConfig, "../../testdata/generic/used-config.yaml", ""},

		{"../../testdata/functional_options", spec.DefaultMuxConfig, "",
			"LazyTree resolves MORE than eager: module handlers' response bodies (map[string]string via " +
				"interface-dispatched Encode) resolve under lazy; eager emits 'no response found' placeholders"},

		{"../../testdata/complex_chi_router", spec.DefaultChiConfig, "", ""},

		{"../../testdata/another_chi_router", spec.DefaultChiConfig, "",
			"the fixture genuinely mounts the same sub-router at BOTH / and /v1; eager only reaches the " +
				"/ mount (/api/*), lazy reaches both and dropSubsumedMountPrefixes keeps the fuller chain " +
				"(/api/v1/*) — each tree shows one of two real prefixes"},
	}

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    engine.DefaultMaxNodesPerTree,
		MaxChildrenPerNode: engine.DefaultMaxChildrenPerNode,
		MaxArgsPerFunction: engine.DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth: engine.DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:  engine.DefaultMaxRecursionDepth,
	}
	genCfg := intspec.GeneratorConfig{OpenAPIVersion: "3.1.1", Title: "parity", APIVersion: "0.0.0"}

	identical := 0
	for _, fx := range fixtures {
		meta := fixtureMetadata(t, fx.dir)

		apiCfg := fx.cfg()
		if fx.cfgFile != "" {
			loaded, err := spec.LoadAPISpecConfig(fx.cfgFile)
			if err != nil {
				t.Fatalf("%s: load %s: %v", fx.dir, fx.cfgFile, err)
			}
			apiCfg = loaded
		}

		eager := intspec.NewTrackerTree(meta, limits, nil)
		lazy := intspec.NewLazyTree(meta, limits)

		eagerSpec, _, err := intspec.MapMetadataToOpenAPIWithDiagnostics(eager, apiCfg, genCfg)
		if err != nil {
			t.Fatalf("%s eager: %v", fx.dir, err)
		}
		lazySpec, _, err := intspec.MapMetadataToOpenAPIWithDiagnostics(lazy, apiCfg, genCfg)
		if err != nil {
			t.Fatalf("%s lazy: %v", fx.dir, err)
		}

		eb, _ := json.Marshal(eagerSpec)
		lb, _ := json.Marshal(lazySpec)
		same := string(eb) == string(lb)

		var missing, extra []string
		for p := range eagerSpec.Paths {
			if _, ok := lazySpec.Paths[p]; !ok {
				missing = append(missing, p)
			}
		}
		for p := range lazySpec.Paths {
			if _, ok := eagerSpec.Paths[p]; !ok {
				extra = append(extra, p)
			}
		}
		sort.Strings(missing)
		sort.Strings(extra)

		switch {
		case same && fx.knownDiff == "":
			identical++
			t.Logf("%-38s IDENTICAL paths=%d", fx.dir, len(eagerSpec.Paths))
		case same && fx.knownDiff != "":
			// A known diff converged: promote it to the must-match list.
			t.Errorf("%s: now IDENTICAL — remove its knownDiff entry", fx.dir)
		case !same && fx.knownDiff != "":
			t.Logf("%-38s KNOWN-DIFF paths eager=%d lazy=%d missing=%v extra=%v — %s",
				fx.dir, len(eagerSpec.Paths), len(lazySpec.Paths), missing, extra, fx.knownDiff)
		default:
			t.Errorf("%s: LazyTree output diverged from eager (paths eager=%d lazy=%d missing=%v extra=%v)",
				fx.dir, len(eagerSpec.Paths), len(lazySpec.Paths), missing, extra)
		}
	}
	t.Logf("parity: %d/%d fixtures byte-identical", identical, len(fixtures))
}
