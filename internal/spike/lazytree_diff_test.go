// LazyTree parity harness — docs/TRACKER_REDESIGN.md step 4.
//
// Runs the full metadata→OpenAPI mapping twice per fixture — once with the
// eager TrackerTree, once with the LazyTree — and reports the diff. This is
// the acceptance instrument for step 4: LazyTree replaces the eager tree
// only when this harness reports parity across all fixtures. Report-only for
// now (it logs, it does not fail), because the LazyTree deliberately does not
// yet implement the mutation overlays (assignments/params/chains/interface
// attachment).
package spike_test

import (
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

func TestLazyTreeParity(t *testing.T) {
	fixtures := []struct {
		dir string
		cfg func() *spec.APISpecConfig
	}{
		{"../../testdata/mux_path_params", spec.DefaultMuxConfig},
		{"../../testdata/mux", spec.DefaultMuxConfig},
		{"../../testdata/chi", spec.DefaultChiConfig},
		{"../../testdata/another_chi_router", spec.DefaultChiConfig},
		{"../../testdata/complex_chi_router", spec.DefaultChiConfig},
		{"../../testdata/gin", spec.DefaultGinConfig},
		{"../../testdata/echo", spec.DefaultEchoConfig},
		{"../../testdata/echo_handler_factory", spec.DefaultEchoConfig},
		{"../../testdata/fiber", spec.DefaultFiberConfig},
		{"../../testdata/servemux", spec.DefaultHTTPConfig},
		{"../../testdata/wrapped_response", spec.DefaultHTTPConfig},
		{"../../testdata/helper_response_body", spec.DefaultHTTPConfig},
	}

	limits := metadata.TrackerLimits{
		MaxNodesPerTree:    engine.DefaultMaxNodesPerTree,
		MaxChildrenPerNode: engine.DefaultMaxChildrenPerNode,
		MaxArgsPerFunction: engine.DefaultMaxArgsPerFunction,
		MaxNestedArgsDepth: engine.DefaultMaxNestedArgsDepth,
		MaxRecursionDepth:  engine.DefaultMaxRecursionDepth,
	}
	genCfg := intspec.GeneratorConfig{OpenAPIVersion: "3.1.1", Title: "parity", APIVersion: "0.0.0"}

	type result struct {
		fixture               string
		eagerPaths, lazyPaths int
		missing, extra        []string
		identical             bool
	}
	var results []result

	for _, fx := range fixtures {
		meta := fixtureMetadata(t, fx.dir)

		eager := intspec.NewTrackerTree(meta, limits, nil)
		lazy := intspec.NewLazyTree(meta, limits)

		eagerSpec, _, err := intspec.MapMetadataToOpenAPIWithDiagnostics(eager, fx.cfg(), genCfg)
		if err != nil {
			t.Fatalf("%s eager: %v", fx.dir, err)
		}
		lazySpec, _, err := intspec.MapMetadataToOpenAPIWithDiagnostics(lazy, fx.cfg(), genCfg)
		if err != nil {
			t.Fatalf("%s lazy: %v", fx.dir, err)
		}

		eb, _ := json.Marshal(eagerSpec)
		lb, _ := json.Marshal(lazySpec)

		r := result{
			fixture:    fx.dir,
			eagerPaths: len(eagerSpec.Paths),
			lazyPaths:  len(lazySpec.Paths),
			identical:  string(eb) == string(lb),
		}
		for p := range eagerSpec.Paths {
			if _, ok := lazySpec.Paths[p]; !ok {
				r.missing = append(r.missing, p)
			}
		}
		for p := range lazySpec.Paths {
			if _, ok := eagerSpec.Paths[p]; !ok {
				r.extra = append(r.extra, p)
			}
		}
		sort.Strings(r.missing)
		sort.Strings(r.extra)
		results = append(results, r)
	}

	parity := 0
	for _, r := range results {
		status := "DIFF"
		if r.identical {
			status = "IDENTICAL"
			parity++
		}
		line := fmt.Sprintf("%-38s %-9s paths eager=%d lazy=%d", r.fixture, status, r.eagerPaths, r.lazyPaths)
		if len(r.missing) > 0 {
			line += fmt.Sprintf(" missing=%v", r.missing)
		}
		if len(r.extra) > 0 {
			line += fmt.Sprintf(" extra=%v", r.extra)
		}
		t.Log(line)
	}
	t.Logf("parity: %d/%d fixtures byte-identical", parity, len(results))
}
