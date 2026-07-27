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
//
// Three ways the function value gets into the field, all of which occur in real
// projects and each recorded differently in metadata (issue #143):
//
//   1. a literal nested inside a function's own literal -> runWeb   (/users)
//   2. a package-level var holding the command, gitea's form -> runAdmin
//      (/admin/stats)
//   3. an inline closure as the Action                              (/metrics)

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

// CmdAdmin mirrors gitea's own shape: the command literal lives in a
// package-level var (`var CmdWeb = &cli.Command{Action: runWeb}`), which is
// recorded differently from a literal written inside a function.
var CmdAdmin = &Command{Name: "admin", Action: runAdmin}

// runAdmin registers the admin surface. Reached only through CmdAdmin's Action.
func runAdmin() error {
	r := chi.NewRouter()
	r.Get("/admin/stats", adminStats)
	return http.ListenAndServe(":8081", r)
}

func adminStats(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Stats{Users: 1})
}

// Stats is the admin resource.
type Stats struct {
	Users int `json:"users"`
}

func main() {
	app := &App{Commands: []*Command{
		{Name: "web", Action: runWeb},
		CmdAdmin,
		// An inline closure is the other common urfave/cli form: the Action is
		// a func literal rather than a named function.
		{Name: "metrics", Action: func() error {
			r := chi.NewRouter()
			r.Get("/metrics", metricsHandler)
			return http.ListenAndServe(":8082", r)
		}},
	}}
	_ = app.Run("web")
}

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Stats{Users: 2})
}
