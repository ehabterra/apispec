// Routers mounted from a struct field, where the field is filled three ways.
// The prefix reaches the mounted routes only if the field's value is traced
// back to the call that built the router.
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type App struct {
	opt chi.Router // filled by a functional option (a closure)
	set chi.Router // filled by a setter method
	lit chi.Router // filled in a struct literal
}

func NewApp(options ...func(*App)) *App {
	app := &App{lit: LitAPI()}
	for _, option := range options {
		option(app)
	}
	return app
}

// WithOpt stores its argument from inside the closure it returns.
func WithOpt(r chi.Router) func(*App) {
	return func(app *App) {
		app.opt = r
	}
}

// SetSet stores its argument directly.
func (a *App) SetSet(r chi.Router) {
	a.set = r
}

func (a *App) Routes() *chi.Mux {
	root := chi.NewRouter()
	root.Mount("/opt", a.opt)
	root.Mount("/set", a.set)
	root.Mount("/lit", a.lit)
	return root
}

func OptAPI() chi.Router {
	r := chi.NewRouter()
	r.Get("/api/items", listItems)
	return r
}

func SetAPI() chi.Router {
	r := chi.NewRouter()
	r.Get("/api/users", listUsers)
	return r
}

func LitAPI() chi.Router {
	r := chi.NewRouter()
	r.Get("/api/orders", listOrders)
	return r
}

func listItems(w http.ResponseWriter, _ *http.Request)  { w.WriteHeader(http.StatusOK) }
func listUsers(w http.ResponseWriter, _ *http.Request)  { w.WriteHeader(http.StatusOK) }
func listOrders(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// Direct returns its literal without a variable in between.
type Direct struct{ r chi.Router }

func NewDirect() *Direct { return &Direct{r: DirectAPI()} }

func (d *Direct) Mount(root *chi.Mux) { root.Mount("/direct", d.r) }

// Value is built as a value literal, not a pointer, and read on a value
// receiver.
type Value struct{ r chi.Router }

func NewValue() Value { v := Value{r: ValueAPI()}; return v }

func (v Value) Mount(root *chi.Mux) { root.Mount("/value", v.r) }

// Conv receives its routers through a type conversion, which is not the call
// that built them: in a literal, and in an explicit store.
type Conv struct{ lit, set chi.Router }

func NewConv() *Conv {
	c := &Conv{lit: chi.Router(ConvLitAPI())}
	c.set = chi.Router(ConvSetAPI())
	return c
}

func (c *Conv) Mount(root *chi.Mux) {
	root.Mount("/convlit", c.lit)
	root.Mount("/convset", c.set)
}

func ConvLitAPI() *chi.Mux { r := chi.NewRouter(); r.Get("/api/convlit", listItems); return r }
func ConvSetAPI() *chi.Mux { r := chi.NewRouter(); r.Get("/api/convset", listItems); return r }

// Local is built in main itself, whose assignments are recorded on a
// different path from a callee's.
type Local struct{ r chi.Router }

func DirectAPI() chi.Router { r := chi.NewRouter(); r.Get("/api/direct", listItems); return r }
func ValueAPI() chi.Router  { r := chi.NewRouter(); r.Get("/api/value", listItems); return r }
func LocalAPI() chi.Router  { r := chi.NewRouter(); r.Get("/api/local", listItems); return r }

func main() {
	app := NewApp(WithOpt(OptAPI()))
	app.SetSet(SetAPI())
	root := app.Routes()
	NewDirect().Mount(root)
	NewValue().Mount(root)
	NewConv().Mount(root)
	local := &Local{r: LocalAPI()}
	root.Mount("/local", local.r)
	_ = http.ListenAndServe(":8080", root)
}
