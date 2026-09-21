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

func main() {
	app := NewApp(WithOpt(OptAPI()))
	app.SetSet(SetAPI())
	_ = http.ListenAndServe(":8080", app.Routes())
}
