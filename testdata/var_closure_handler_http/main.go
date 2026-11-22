// A handler closure held in a local variable is the handler (issue #610),
// on plain net/http: both the short form and a declared http.HandlerFunc.
package main

import (
	"encoding/json"
	"net/http"
	"os"
)

type Geo struct {
	Lat float64 `json:"lat"`
}

type Status struct {
	OK bool `json:"ok"`
}

type Inner struct {
	X int `json:"x"`
}

type Pkg struct {
	Name string `json:"name"`
}

// A package-level function sharing its name with a local below.
func pkgHandler(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Pkg{}) }

// The inner block's `pkgHandler := func…` is the only recorded assignment to the
// name, yet the outer registration names the PACKAGE-LEVEL function: it must
// not be attributed to the inner closure (an assignment must dominate the call).
func shadowPkg(mux *http.ServeMux) {
	if os.Getenv("SHADOW") != "" {
		pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }
		mux.HandleFunc("GET /shadow/inner", pkgHandler)
	}
	mux.HandleFunc("GET /shadow/outer", pkgHandler)
}

// The same with a parameter as the outer binding.
func shadowParam(mux *http.ServeMux, p http.HandlerFunc) {
	if os.Getenv("SHADOW") != "" {
		p := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }
		mux.HandleFunc("GET /shadowp/inner", p)
	}
	mux.HandleFunc("GET /shadowp/outer", p)
}

// In a METHOD body the closure resolves as it does in a function: methods
// record their control-flow regions too (issue #617), so the assignment is
// proven to be the binding the call names.
type API struct{ mux *http.ServeMux }

func (a *API) routes() {
	m := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Geo{}) }
	a.mux.HandleFunc("GET /method", m)
}

// The shadowing shape in a METHOD: the outer registration names the parameter,
// not the inner block's closure.
func (a *API) methodShadow(g http.HandlerFunc) {
	if os.Getenv("SHADOW") != "" {
		g := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }
		a.mux.HandleFunc("GET /mshadow/inner", g)
	}
	a.mux.HandleFunc("GET /mshadow/outer", g)
}

// The package-level shadow in a METHOD: no parameter hop to fall back on, so
// only the method's recorded regions keep the outer call off the inner closure.
func (a *API) methodShadowPkg() {
	if os.Getenv("SHADOW") != "" {
		pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }
		a.mux.HandleFunc("GET /mshadowpkg/inner", pkgHandler)
	}
	a.mux.HandleFunc("GET /mshadowpkg/outer", pkgHandler)
}

// The closure reaches the registration through a helper's parameter: the
// parameter hop and the closure hop chain.
func register(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, h)
}

func main() {
	mux := http.NewServeMux()

	h := func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("count")
		json.NewEncoder(w).Encode(Geo{})
	}
	mux.HandleFunc("GET /geo", h)

	var health http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Status{})
	}
	mux.Handle("GET /health", health)

	wrapped := func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("page")
		json.NewEncoder(w).Encode(Status{})
	}
	register(mux, "GET /wrapped", wrapped)

	// Converted at the registration: http.HandlerFunc(c) is looked through.
	c := func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("q")
		json.NewEncoder(w).Encode(Geo{})
	}
	mux.Handle("GET /converted", http.HandlerFunc(c))

	shadowPkg(mux)
	shadowParam(mux, health)
	api := &API{mux: mux}
	api.routes()
	api.methodShadow(pkgHandler)
	api.methodShadowPkg()

	// The control: inline.
	mux.HandleFunc("GET /inline", func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("count")
		json.NewEncoder(w).Encode(Geo{})
	})

	initScopeIf(mux)
	initScopeSwitch(mux)
	initScopeTypeSwitch(mux)
	initScopeFor(mux)
	(&API{mux: mux}).initScopeMethod()

	http.ListenAndServe(":8080", mux)
}

// A closure declared in a statement's initializer is visible in the statement,
// every else arm included, and nowhere after it: the registration after the
// statement names the package-level function (issue #618). The init runs
// unconditionally, so it dominates that registration — scope, not dominance,
// is what tells the two bindings apart.
func initScopeIf(mux *http.ServeMux) {
	if pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }; os.Getenv("A") != "" {
		mux.HandleFunc("GET /ifinit/inner", pkgHandler)
	} else {
		mux.HandleFunc("GET /ifinit/else", pkgHandler)
	}
	mux.HandleFunc("GET /ifinit/outer", pkgHandler)
}

func initScopeSwitch(mux *http.ServeMux) {
	switch pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }; os.Getenv("A") {
	case "x":
		mux.HandleFunc("GET /swinit/inner", pkgHandler)
	}
	mux.HandleFunc("GET /swinit/outer", pkgHandler)
}

func initScopeTypeSwitch(mux *http.ServeMux) {
	var v any = 1
	switch pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }; v.(type) {
	case int:
		mux.HandleFunc("GET /tsinit/inner", pkgHandler)
	}
	mux.HandleFunc("GET /tsinit/outer", pkgHandler)
}

func initScopeFor(mux *http.ServeMux) {
	for pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }; ; {
		mux.HandleFunc("GET /forinit/inner", pkgHandler)
		break
	}
	mux.HandleFunc("GET /forinit/outer", pkgHandler)
}

// The same in a METHOD body, which records its regions as a function does.
func (a *API) initScopeMethod() {
	if pkgHandler := func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(Inner{}) }; os.Getenv("A") != "" {
		a.mux.HandleFunc("GET /minit/inner", pkgHandler)
	}
	a.mux.HandleFunc("GET /minit/outer", pkgHandler)
}
