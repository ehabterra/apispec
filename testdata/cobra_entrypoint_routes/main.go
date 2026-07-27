// Fixture: the cobra half of issue #220 — routes reachable only through
// cobra's dispatcher, which lives in github.com/spf13/cobra.
//
// Depends on the REAL cobra so the dispatcher genuinely sits in a package
// apispec never analyses: `root.Execute()` reaches `RunE` through cobra's own
// internals, so nothing in this module calls runServe. The whole route subtree
// exists only if the RunE/Run fields are treated as entrypoints.
package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"
)

type Widget struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// serveCmd is the routing entrypoint: cobra's most common shape.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "starts the HTTP server",
	RunE:  runServe,
}

// migrateCmd is the control — a command that registers no routes must not be
// rooted, or every subcommand of every cobra program would drag its subtree in.
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "runs migrations",
	RunE:  runMigrate,
}

// versionCmd uses Run rather than RunE, cobra's other command body, so both
// fields are covered end to end.
var versionCmd = &cobra.Command{
	Use: "version",
	Run: runVersion,
}

func runServe(_ *cobra.Command, _ []string) error {
	r := chi.NewRouter()
	r.Get("/widgets", listWidgets)
	r.Post("/widgets", createWidget)
	return http.ListenAndServe(":8080", r)
}

func runMigrate(_ *cobra.Command, _ []string) error {
	_ = os.Getenv("DB_DSN")
	return nil
}

func runVersion(_ *cobra.Command, _ []string) {
	r := chi.NewRouter()
	r.Get("/version", showVersion)
	_ = http.ListenAndServe(":8081", r)
}

func listWidgets(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode([]Widget{})
}

func createWidget(w http.ResponseWriter, r *http.Request) {
	var wid Widget
	_ = json.NewDecoder(r.Body).Decode(&wid)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(wid)
}

func showVersion(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Widget{Name: "1.0.0"})
}

func main() {
	root := &cobra.Command{Use: "app"}
	root.AddCommand(serveCmd, migrateCmd, versionCmd)
	_ = root.Execute()
}
