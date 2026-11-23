// Fixture: a project router type wrapping chi, whose Mount method forwards the
// sub-router's FIELD to chi (issue #611):
//
//	func (r *Router) Mount(pattern string, sub *Router) {
//		r.chiRouter.Mount(r.getPattern(pattern), sub.chiRouter)
//	}
//
// The sub-router's routes are registered on `m.chiRouter` from inside the
// wrapper's Get, below the call that built `m` (APIRoutes()). The Mount node
// sits in the wrapper's body and receives `sub.chiRouter` — a field of a
// parameter — so the prefix has to travel from the field, through the
// parameter, to the caller's argument and the subtree that produced it. gitea
// mounts its whole API this way.
//
// Every wrapper Mount is exercised with BOTH an inline call argument and a
// variable argument, because assigning the value first often makes otherwise
// identical code resolve (golden rule 11):
//
//	/api/v1, /api/v2      Mount: the pattern goes through r.getPattern
//	/api/plain, /api/pv   MountPlain: the pattern is forwarded untouched
//	/api/mux, /api/mx     MountMux: takes *chi.Mux directly — the control
//
// Two negative shapes ride along: OrphanRoutes builds a wrapper router whose
// routes are registered but which is never mounted (its route stays at the
// root, no prefix is invented), and each sub-router is mounted under exactly
// one prefix (its routes must not leak under another's).
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router wraps chi the way gitea's modules/web.Router does.
type Router struct {
	chiRouter *chi.Mux
	prefix    string
}

func NewRouter() *Router { return &Router{chiRouter: chi.NewRouter()} }

func (r *Router) getPattern(p string) string { return r.prefix + p }

func (r *Router) Get(pattern string, h http.HandlerFunc) {
	r.chiRouter.Get(r.getPattern(pattern), h)
}

// Mount prefixes the pattern before forwarding the sub-router's chi field.
func (r *Router) Mount(pattern string, sub *Router) {
	r.chiRouter.Mount(r.getPattern(pattern), sub.chiRouter)
}

// MountPlain forwards the pattern untouched.
func (r *Router) MountPlain(pattern string, sub *Router) {
	r.chiRouter.Mount(pattern, sub.chiRouter)
}

// MountMux takes the chi router directly.
func (r *Router) MountMux(pattern string, sub *chi.Mux) {
	r.chiRouter.Mount(pattern, sub)
}

type User struct {
	Name string `json:"name"`
}

type Repo struct {
	Slug string `json:"slug"`
}

func getUser(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(User{}) }
func getRepo(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(Repo{}) }

func APIRoutes() *Router    { m := NewRouter(); m.Get("/user", getUser); return m }
func APIRoutes2() *Router   { m := NewRouter(); m.Get("/user2", getUser); return m }
func PlainRoutes() *Router  { m := NewRouter(); m.Get("/repo", getRepo); return m }
func PlainRoutes2() *Router { m := NewRouter(); m.Get("/repo2", getRepo); return m }
func MuxRoutes() *chi.Mux   { m := chi.NewRouter(); m.Get("/mux", getUser); return m }
func MuxRoutes2() *chi.Mux  { m := chi.NewRouter(); m.Get("/mux2", getUser); return m }

// OrphanRoutes builds a wrapper router and registers on it directly, at the
// root; it is never mounted.
func OrphanRoutes(r *Router) { r.Get("/orphan", getRepo) }

func main() {
	r := NewRouter()

	api := APIRoutes()
	r.Mount("/api/v1", api)          // variable argument
	r.Mount("/api/v2", APIRoutes2()) // inline call argument

	plain := PlainRoutes()
	r.MountPlain("/api/plain", plain)       // variable argument
	r.MountPlain("/api/pv", PlainRoutes2()) // inline call argument

	mx := MuxRoutes2()
	r.MountMux("/api/mux", MuxRoutes()) // inline call argument
	r.MountMux("/api/mx", mx)           // variable argument

	OrphanRoutes(r)

	_ = http.ListenAndServe(":8080", r.chiRouter)
}
