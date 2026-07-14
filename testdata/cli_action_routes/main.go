package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// This fixture mirrors the urfave/cli wiring style (gitea, many CLIs):
// route registration is reachable from main ONLY through a function value
// stored in a composite-literal field, invoked by a dispatcher. Static
// call-graph tracing stops at app.Run — there is no direct call edge from
// main's subtree to runWeb.

// Command mirrors cli.Command: the handler wiring hides in Action.
type Command struct {
	Name   string
	Action func() error
}

// App mirrors cli.App.
type App struct {
	Commands []*Command
}

// Run dispatches to the named command's Action — the dynamic hop.
func (a *App) Run(name string) error {
	for _, c := range a.Commands {
		if c.Name == name {
			return c.Action()
		}
	}
	return nil
}

// User is the API resource.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode([]User{})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	_ = json.NewDecoder(r.Body).Decode(&u)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(u)
}

// runWeb registers the routes — plain chi, no wrapper router, so the ONLY
// obstacle between main and these registrations is the Action field hop.
func runWeb() error {
	r := chi.NewRouter()
	r.Get("/users", listUsers)
	r.Post("/users", createUser)
	return http.ListenAndServe(":8080", r)
}

func main() {
	app := &App{Commands: []*Command{{Name: "web", Action: runWeb}}}
	_ = app.Run("web")
}
