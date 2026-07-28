// Fixture: route registration reachable ONLY through a command library's
// dispatcher (issue #220) — the shape that made apispec report zero routes for
// gitea and photoprism.
//
// This depends on the REAL github.com/urfave/cli/v3 on purpose. The dispatcher
// that calls `Action` lives inside that module, which apispec never analyses, so
// no call edge from main reaches runWeb: the whole registration subtree exists
// only if the Action field is treated as an entrypoint. A hand-written in-module
// dispatcher (testdata/cli_action_routes) cannot stand in for this — it has an
// Action call edge, and that is exactly the difference that made the first
// attempt at #143 look fixed while gitea stayed at zero.
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/urfave/cli/v3"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CmdWeb holds the routing entrypoint, gitea's exact shape.
var CmdWeb = &cli.Command{
	Name:   "web",
	Usage:  "starts the web server",
	Action: runWeb,
}

// CmdIndex is the control: an entrypoint that registers no routes must NOT be
// rooted, or every subcommand of every CLI would drag its subtree into the walk.
var CmdIndex = &cli.Command{
	Name:   "index",
	Usage:  "indexes files",
	Action: runIndex,
}

func runWeb(_ context.Context, _ *cli.Command) error {
	r := chi.NewRouter()
	r.Get("/users", listUsers)
	r.Post("/users", createUser)
	return http.ListenAndServe(":8080", r)
}

func runIndex(_ context.Context, _ *cli.Command) error {
	_ = os.Getenv("INDEX_PATH")
	return nil
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

func main() {
	app := &cli.Command{
		Name:     "app",
		Commands: []*cli.Command{CmdWeb, CmdIndex},
	}
	_ = app.Run(context.Background(), os.Args)
}
