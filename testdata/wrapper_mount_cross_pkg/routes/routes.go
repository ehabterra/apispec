// Package routes builds the sub-routers the main package mounts.
package routes

import (
	"encoding/json"
	"net/http"

	"github.com/ehabterra/apispec/testdata/wrapper_mount_cross_pkg/web"
)

type User struct {
	Name string `json:"name"`
}

func getUser(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(User{}) }

func APIRoutes() *web.Router    { m := web.NewRouter(); m.Get("/user", getUser); return m }
func HealthRoutes() *web.Router { m := web.NewRouter(); m.Get("/h", getUser); return m }
func StatusRoutes() *web.Router { m := web.NewRouter(); m.Get("/s", getUser); return m }
func FixedRoutes() *web.Router  { m := web.NewRouter(); m.Get("/f", getUser); return m }

// SameRoutes returns a local named like the caller's variable (`same :=
// routes.SameRoutes()`).
func SameRoutes() *web.Router { same := web.NewRouter(); same.Get("/same", getUser); return same }
