// Fixture: a house router's Mount methods called across packages, each with a
// variable argument (issue #611, review follow-ups):
//
//	/api/v1     Mount: the factory's local is named differently from the variable
//	/api/same   Mount: the factory's local has the variable's own name
//	/api/h      MountWithHealth: an underivable mount precedes the forwarding one
//	/api/s      MountWithStatus: the forwarding mount precedes a fixed one
//	/v1         MountFixed: the wrapper adds "/fixed" to the pattern
//
// The fixed /health and /status routers are mounted on the receiver inside the
// wrapper's body, so they stay where the receiver is, not under the caller's
// prefix.
package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/wrapper_mount_cross_pkg/routes"
	"github.com/ehabterra/apispec/testdata/wrapper_mount_cross_pkg/web"
)

func main() {
	r := web.NewRouter()

	api := routes.APIRoutes()
	r.Mount("/api/v1", api)

	same := routes.SameRoutes()
	r.Mount("/api/same", same)

	hr := routes.HealthRoutes()
	r.MountWithHealth("/api/h", hr)

	sr := routes.StatusRoutes()
	r.MountWithStatus("/api/s", sr)

	fx := routes.FixedRoutes()
	r.MountFixed("/v1", fx)

	_ = http.ListenAndServe(":8080", r.ChiRouter)
}
