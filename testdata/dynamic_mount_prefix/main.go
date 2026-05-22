// Reproduction of issue #34 — dynamic mount prefix computed via a helper.
//
// At runtime mountPoint() collapses to "/api" when prefix is empty, so the
// expected routes are /api, /api/{id}, /api/changepassword, etc. Today
// APISpec can't evaluate the helper call and falls back to rendering the
// call expression as text — producing routes prefixed with something like
// /dynamic_mount_prefix.mountPoint/...
package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountPoint mirrors the helper from issue #34: joins a configurable
// prefix with a sub-path. Returns "/api" when prefix is empty.
func mountPoint(prefix, point string) string {
	if prefix == "" && point == "" {
		return "/"
	}

	points := make([]string, 1, 10)

	for _, v := range strings.Split(prefix, "/") {
		if v != "" {
			points = append(points, v)
		}
	}
	for _, v := range strings.Split(point, "/") {
		if v != "" {
			points = append(points, v)
		}
	}
	if len(points) == 1 {
		return "/"
	}
	return strings.Join(points, "/")
}

// apiRoutes returns a sub-router with several routes under /api.
func apiRoutes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("api root"))
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		fmt.Println(id)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("api item"))
	})

	r.Post("/changepassword", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	r.Delete("/clear", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("cleared"))
	})

	return r
}

func main() {
	// prefix is empty here, so mountPoint(prefix, "/api") == "/api".
	prefix := ""

	r := chi.NewRouter()
	r.Mount(mountPoint(prefix, "/api"), apiRoutes())

	// A literal-mount sibling, to make sure the regression of the
	// dynamic-mount case doesn't drag the literal case down with it.
	// Same sub-router (apiRoutes) mounted at a second, literal path. With
	// the per-node visited map this second traversal short-circuited and
	// none of /v2/api's routes appeared in the spec — see the visited
	// map fix in extractor.go (now keyed by node + mountPath).
	r.Mount("/v2/api", apiRoutes())

	http.ListenAndServe(":8080", r)
}
