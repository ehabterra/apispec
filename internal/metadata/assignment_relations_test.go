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

package metadata

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/typemodel"
)

func metadataFor(t *testing.T, src string) *Metadata {
	t.Helper()
	file, info, fset := sweepTypeCheck(t, src)
	return GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"sweep.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"sweep.go": "p"},
		fset,
	)
}

// An assignment with no call on its right-hand side is not handed to the next
// call the walk meets. `c := &Ctx{}` followed by `c.Do()` recorded c.Do as the
// producer of c (issue #550).
func TestPendingAssignmentDoesNotLeakToNextCall(t *testing.T) {
	meta := metadataFor(t, `package p

type Ctx struct{}

func (c *Ctx) Do() {}

func handler() {
	c := &Ctx{}
	c.Do()
}

func main() { handler() }
`)
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		if meta.StringPool.GetString(e.Callee.Name) == "Do" && e.CalleeRecvVarName != "" {
			t.Errorf("c.Do() recorded as the producer of %q", e.CalleeRecvVarName)
		}
	}
}

// A variable assigned inside a function with no call behind it has no
// producer. It used to be linked to the INVOCATION of the function it is
// written in — for a handler closure, the route registration — which made the
// registration's whole subtree the variable's value (issue #550).
func TestAssignmentWithoutProducingCallHasNoRelation(t *testing.T) {
	meta := metadataFor(t, `package p

type Req struct{ Path string }

func save(string) {}

func register(r *Req) {
	name := r.Path
	save(name)
}

func main() { register(&Req{}) }
`)
	for key, rel := range meta.BuildAssignmentRelationships() {
		if key.Name != "name" {
			continue
		}
		t.Errorf("name is linked to %q, but no call produced it",
			meta.StringPool.GetString(rel.Edge.Callee.Name))
	}
}

// The case the fallback exists for keeps working: an assignment in the
// CALLER's own scope, recorded on the edge that produced it — `main`'s
// `g := newGroup()`, every variable of a tuple included.
func TestCallerScopeAssignmentKeepsItsProducer(t *testing.T) {
	meta := metadataFor(t, `package p

type Group struct{}

func newGroup() (*Group, error) { return &Group{}, nil }

func main() {
	g, err := newGroup()
	_, _ = g, err
}
`)
	linked := map[string]string{}
	for key, rel := range meta.BuildAssignmentRelationships() {
		linked[key.Name] = meta.StringPool.GetString(rel.Edge.Callee.Name)
	}
	for _, v := range []string{"g", "err"} {
		if linked[v] != "newGroup" {
			t.Errorf("%s linked to %q, want its producing newGroup() call; have %v", v, linked[v], linked)
		}
	}
}

// fieldProducers returns, for every relation whose key names a struct field,
// the callee of the edge that produces it.
func fieldProducers(meta *Metadata) map[string]string {
	out := map[string]string{}
	for key, rel := range meta.BuildAssignmentRelationships() {
		out[key.Name] = meta.StringPool.GetString(rel.Edge.Callee.Name)
	}
	return out
}

// A router stored in a struct field keeps its producer when the value stored
// IS a parameter the invoking call binds: the functional-options closure and
// the plain setter. #550 dropped every callee-body assignment without a call on
// its right-hand side, and with them these two, so a router mounted from the
// field lost its prefix on every route it registered — found and documented at
// the root, which no count of routes notices.
func TestFieldStoreOfBoundParameterKeepsProducer(t *testing.T) {
	got := fieldProducers(metadataFor(t, `package p

type Router interface{ Get() }
type mux struct{}

func (*mux) Get() {}

type App struct{ opt, set Router }

func NewApp(options ...func(*App)) *App {
	app := &App{}
	for _, option := range options {
		option(app)
	}
	return app
}

func WithOpt(r Router) func(*App) { return func(app *App) { app.opt = r } }

func (a *App) SetSet(r Router) { a.set = r }

func API() Router { return &mux{} }

func main() {
	app := NewApp(WithOpt(API()))
	app.SetSet(API())
}
`))
	for field, want := range map[string]string{"p.App.opt": "WithOpt", "p.App.set": "SetSet"} {
		if got[field] != want {
			t.Errorf("%s produced by %q, want the %s call that bound its argument; have %v", field, got[field], want, got)
		}
	}
}

// The parameter rule links only the parameter ITSELF, bound by the call that
// is the edge. Each negative here is a shape #550 exists to keep unlinked.
func TestFieldStoreOfParameterStaysHonest(t *testing.T) {
	cases := map[string]string{
		// A value DERIVED from a parameter is not the argument.
		"derived": `package p

type Req struct{ Path string }
type Store struct{ path string }

func (s *Store) Keep(r *Req) { s.path = r.Path }

func main() { (&Store{}).Keep(&Req{}) }
`,
		// A closure parameter shadowing the outer one: the stored r is the
		// closure's *Req, not the Router the registration was given.
		"shadowed": `package p

type Router interface{ Handle(func(*Req)) }
type Req struct{}
type Store struct{ last *Req }

var s Store

func Register(r Router) {
	r.Handle(func(r *Req) { s.last = r })
}

func main() { Register(nil) }
`,
		// A name the edge does not bind is not an argument of that call.
		"unbound": `package p

type Store struct{ n int }

var global = 3

func (s *Store) Reset(x int) { n := global; s.n = n }

func main() { (&Store{}).Reset(1) }
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			for field, producer := range fieldProducers(metadataFor(t, src)) {
				if strings.Contains(field, "Store.") {
					t.Errorf("%s linked to %q, but the stored value is not an argument that call bound", field, producer)
				}
			}
		})
	}
}

// A signature records a parameter's type as declared (`Router`); an
// identifier's is package-qualified. The comparison has to see those as the
// same type, or the rule never fires on a real project — which is how the
// first version of this fix passed a single-package test and still left every
// cross-package mount without its prefix.
func TestSameDeclaredType(t *testing.T) {
	ref := typemodel.Parse
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"Router", "github.com/go-chi/chi/v5.Router", true},
		{"*App", "*example.com/app.App", true},
		{"Router", "Router", true},
		{"example.com/a.Router", "example.com/b.Router", false},
		{"*Req", "Router", false},
		{"Router", "*github.com/go-chi/chi/v5.Router", false},
		{"[]Router", "[]github.com/go-chi/chi/v5.Router", true},
		{"map[string]Router", "map[int]Router", false},
	} {
		if got := sameDeclaredType(ref(tc.a), ref(tc.b)); got != tc.want {
			t.Errorf("sameDeclaredType(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// A struct literal's keyed call element is the same fact as an explicit store
// into that field, and has to be recorded under the same key: the tracker
// looks a mounted field up by the name, field type and receiver type an
// explicit `x.f = …` renders, so a literal store rendered any other way would
// miss that lookup silently (issue #565).
func TestLiteralFieldStoreMatchesExplicitStore(t *testing.T) {
	meta := metadataFor(t, `package p

type Router interface{ Get() }
type App struct{ lit, set Router }
type Val struct{ lit, set Router }

func API() Router { return nil }

func NewApp() *App {
	app := &App{lit: API()}
	app.set = API()
	return app
}

func NewVal() Val {
	v := Val{lit: API()}
	v.set = API()
	return v
}

func main() { NewApp(); NewVal() }
`)
	stores := map[string]*Assignment{}
	for i := range meta.CallGraph {
		for k, a := range meta.CallGraph[i].AssignmentMap {
			stores[k] = &a[len(a)-1]
		}
	}
	for _, pair := range [][2]string{{"p.App.lit", "p.App.set"}, {"Val.lit", "Val.set"}} {
		lit, set := stores[pair[0]], stores[pair[1]]
		if lit == nil || set == nil {
			t.Errorf("%s / %s not recorded; have %v", pair[0], pair[1], mapKeys(stores))
			continue
		}
		if got, want := lit.Lhs.X.GetType(), set.Lhs.X.GetType(); got != want {
			t.Errorf("%s: literal store's receiver type %q, explicit store's %q — the lookup keys on it", pair[0], got, want)
		}
		if got, want := meta.StringPool.GetString(lit.ConcreteType), meta.StringPool.GetString(set.ConcreteType); got != want {
			t.Errorf("%s: literal store's field type %q, explicit store's %q", pair[0], got, want)
		}
		if lit.Value.GetKind() != KindCall {
			t.Errorf("%s: literal store's value kind %q, want the call", pair[0], lit.Value.GetKind())
		}
	}
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Each literal shape links its field to the call that built the value: a
// pointer literal assigned or returned, a value literal, and one built in main,
// whose own assignments are recorded on a different path from a callee's.
func TestLiteralFieldStoreKeepsProducer(t *testing.T) {
	got := fieldProducers(metadataFor(t, `package p

type Router interface{ Get() }
type A struct{ r Router }
type B struct{ r Router }
type C struct{ r Router }
type D struct{ r Router }

func MakeA() Router { return nil }
func MakeB() Router { return nil }
func MakeC() Router { return nil }
func MakeD() Router { return nil }

func NewA() *A { a := &A{r: MakeA()}; return a }
func NewB() *B { return &B{r: MakeB()} }
func NewC() C  { return C{r: MakeC()} }

func main() {
	NewA()
	NewB()
	NewC()
	d := &D{r: MakeD()}
	_ = d
}
`))
	// A value (non-pointer) receiver renders without its package — `C.r`, not
	// `p.C.r` — exactly as an explicit store on a value does; parity is what
	// matters, and TestLiteralFieldStoreMatchesExplicitStore holds it.
	for field, want := range map[string]string{"p.A.r": "MakeA", "p.B.r": "MakeB", "C.r": "MakeC", "p.D.r": "MakeD"} {
		if got[field] != want {
			t.Errorf("%s produced by %q, want %s; have %v", field, got[field], want, got)
		}
	}
}

// Only a keyed call element of a STRUCT literal is a field store worth a
// producer. The others are left out, and `&T{…}` — which the walk meets twice,
// as the unary expression and as its operand — is recorded once.
func TestLiteralFieldStoreScope(t *testing.T) {
	meta := metadataFor(t, `package p

type Router interface{ Get() }
type T struct {
	r    Router
	name string
}
type Pair struct{ a, b Router }

func API() Router { return nil }

func Build() {
	_ = &T{r: API(), name: "x"}           // name is a literal, not a call
	_ = Pair{API(), API()}                // positional: no field key
	_ = map[string]Router{"k": API()}     // a map, not a struct
	_ = []Router{API()}                   // a slice
}

func main() { Build() }
`)
	fn := meta.FunctionInPackage("p", "Build")
	if fn == nil {
		t.Fatal("Build not recorded")
	}
	var keys []string
	for k, as := range fn.AssignmentMap {
		if strings.Contains(k, ".") {
			keys = append(keys, k)
			if len(as) != 1 {
				t.Errorf("%s recorded %d times, want once", k, len(as))
			}
		}
	}
	if len(keys) != 1 || keys[0] != "p.T.r" {
		t.Errorf("field stores = %v, want only p.T.r", keys)
	}
}

// A method records its literal field stores in its own assignment map, beside
// the explicit ones, as a function does: consumers resolving a variable inside
// a method body read that map.
func TestLiteralFieldStoreInMethod(t *testing.T) {
	meta := metadataFor(t, `package p

type Router interface{ Get() }
type App struct{ r Router }
type Server struct{}

func API() Router { return nil }

func (s *Server) Build() *App { return &App{r: API()} }

func main() { (&Server{}).Build() }
`)
	var found bool
	for _, file := range meta.Packages["p"].Files {
		for _, typ := range file.Types {
			for _, m := range typ.Methods {
				if meta.StringPool.GetString(m.Name) == "Build" && len(m.AssignmentMap["p.App.r"]) == 1 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("Server.Build's literal store p.App.r not recorded in its assignment map")
	}
}
