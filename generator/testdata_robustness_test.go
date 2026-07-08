package generator

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_RecursiveTypes is the regression fixture for issue #10 (stack
// overflow from a cyclic struct driving unbounded recursion in
// generateStructSchema) and issue #14 (truncated output on the same project).
// The schema mapper must break every type cycle by emitting a $ref to the
// already-registered component instead of expanding it inline forever. A
// regression surfaces either as a hang / stack overflow (this test never
// returns) or as a missing/dangling component (the $ref assertions fail).
func TestTestdata_RecursiveTypes(t *testing.T) {
	out := loadTestdata(t, "recursive_types", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, p := range []string{"/tree", "/category", "/graph"} {
		if !hasPath(out, p) {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}

	// Every cyclic type must be registered as its own component so the cycle
	// can close as a $ref.
	for _, suffix := range []string{"_TreeNode", "_Category", "_Product", "_Graph", "_Edge", "_Node"} {
		if componentByName(out, suffix) == nil {
			t.Errorf("expected component ending in %q; have %v", suffix, mapSchemaKeys(out.Components.Schemas))
		}
	}

	// Direct self-cycle: TreeNode.parent -> TreeNode and
	// TreeNode.children[] -> TreeNode both close as $refs.
	tree := componentByName(out, "_TreeNode")
	if tree == nil {
		t.Fatalf("TreeNode component missing")
	}
	assertPropRefSuffix(t, tree, "parent", "TreeNode")
	assertArrayPropItemsRefSuffix(t, tree, "children", "TreeNode")

	// Mutual cycle: Category.products[] -> Product and Product.category ->
	// Category. Both directions must resolve to a $ref, not inline forever.
	category := componentByName(out, "_Category")
	product := componentByName(out, "_Product")
	if category == nil || product == nil {
		t.Fatalf("Category/Product components missing")
	}
	assertArrayPropItemsRefSuffix(t, category, "products", "Product")
	assertPropRefSuffix(t, product, "category", "Category")
	// Category also nests under a parent Category (self-cycle alongside the
	// mutual one).
	assertPropRefSuffix(t, category, "parent", "Category")
}

// assertPropRefSuffix asserts that schema.Properties[prop] is a $ref whose
// target component name ends with wantSuffix.
func assertPropRefSuffix(t *testing.T, schema *intspec.Schema, prop, wantSuffix string) {
	t.Helper()
	p := schema.Properties[prop]
	if p == nil {
		t.Errorf("property %q missing", prop)
		return
	}
	if p.Ref == "" {
		t.Errorf("property %q should be a $ref (cycle), got %+v", prop, p)
		return
	}
	if !strings.HasSuffix(p.Ref, wantSuffix) {
		t.Errorf("property %q $ref = %q, want suffix %q", prop, p.Ref, wantSuffix)
	}
}

// assertArrayPropItemsRefSuffix asserts that schema.Properties[prop] is an array
// whose items are a $ref ending with wantSuffix.
func assertArrayPropItemsRefSuffix(t *testing.T, schema *intspec.Schema, prop, wantSuffix string) {
	t.Helper()
	p := schema.Properties[prop]
	if p == nil {
		t.Errorf("property %q missing", prop)
		return
	}
	if p.Type != "array" || p.Items == nil {
		t.Errorf("property %q should be an array with items, got %+v", prop, p)
		return
	}
	if p.Items.Ref == "" || !strings.HasSuffix(p.Items.Ref, wantSuffix) {
		t.Errorf("property %q items $ref = %q, want suffix %q", prop, p.Items.Ref, wantSuffix)
	}
}

// TestTestdata_DenseGraphBounded is the regression fixture for issue #20 (hang
// on scan — the tracker's tree expansion was exponential in a dense call
// graph). The dense_graph fixture models that project's shape at a realistic
// scale: many endpoints fanning into a shared service/repo/leaf layer. With the
// tracker's traversal limits in place, generation completes quickly; this test
// asserts it finishes within a generous wall-clock budget so a regression that
// reintroduces unbounded traversal fails loud (via timeout) instead of hanging
// CI indefinitely.
//
// The budget is deliberately generous (local generation is ~1s) so slower CI
// machines never flake, while still being far below the minutes-to-forever an
// unbounded traversal would take.
func TestTestdata_DenseGraphBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping dense-graph stress test in -short mode")
	}

	out := generateWithinBudget(t, "dense_graph", 60*time.Second)
	// All 25 endpoints must survive the dense traversal.
	assertRoutes(t, out, 25)
	noDanglingRefs(t, out)
}

// TestTestdata_CyclicGraphBounded is the regression fixture for issue #20's
// worst case: a dense, strongly-connected (cyclic) call graph. Each web
// function calls three others by modular index, forming many back-edges, so
// shared callees are re-expanded along exponentially many cycle paths. Before
// the cumulative MaxNodesPerTree cap (tracker.go: tree.nodesBuilt), the tracker
// re-expanded these indefinitely — the old len(visited) check measured only the
// live recursion-stack depth, which stays small, so it never fired and
// generation ran effectively forever (>45s with no truncation). The cumulative
// counter caps total nodes built and unwinds the recursion cheaply once hit.
// This test asserts generation now completes within a wall-clock budget while
// still recovering every route.
func TestTestdata_CyclicGraphBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cyclic-graph stress test in -short mode")
	}

	out := generateWithinBudget(t, "cyclic_graph", 60*time.Second)
	// Route detection happens near the call-graph roots, above the dense web,
	// so every handler must still be found even though the deep traversal is
	// truncated.
	assertRoutes(t, out, 12)
	noDanglingRefs(t, out)
}

// generateWithinBudget runs GenerateFromDirectory for a testdata fixture in a
// goroutine and fails the test if it does not finish within budget. It
// deliberately does not wait for the goroutine on timeout: a truly unbounded
// traversal would never return, and failing fast is the whole point of the
// stress test.
func generateWithinBudget(t *testing.T, name string, budget time.Duration) *spec.OpenAPISpec {
	t.Helper()
	dir := filepath.Join("..", "testdata", name)

	type result struct {
		out *spec.OpenAPISpec
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := NewGenerator(spec.DefaultHTTPConfig()).GenerateFromDirectory(dir)
		done <- result{out, err}
	}()

	select {
	case <-time.After(budget):
		t.Fatalf("%s generation exceeded %s budget — traversal is likely unbounded again", name, budget)
		return nil
	case res := <-done:
		if res.err != nil {
			t.Fatalf("GenerateFromDirectory(%s): %v", name, res.err)
		}
		if res.out == nil || res.out.Paths == nil {
			t.Fatalf("nil spec or paths for %s", name)
		}
		return res.out
	}
}

// assertRoutes checks that /route0../route(n-1) are all present.
func assertRoutes(t *testing.T, out *spec.OpenAPISpec, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		p := "/route" + strconv.Itoa(i)
		if !hasPath(out, p) {
			t.Errorf("path %q missing; got %d paths", p, len(out.Paths))
		}
	}
}
