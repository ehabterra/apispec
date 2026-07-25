// Fixture: the urfave/cli wiring style spread across packages, which is how it
// actually looks in the wild (issue #143). Everything here is one hop harder than
// the single-package `cli_action_routes` fixture:
//
//   - the command type is package-qualified (`clipkg.Command{…}`), so the literal
//     names its type through a selector rather than a bare ident;
//   - one Action is a CROSS-PACKAGE function value (`web.RunWeb`) — a selector,
//     not an ident, which is the resolution that silently collapses if the callee
//     name is read from the wrong place (golden rule #10);
//   - one Action is a METHOD value (`srv.RunAdmin`), whose key needs the receiver
//     type as well as the package.
package main

import (
	"github.com/ehabterra/apispec/testdata/cli_action_cross_package/clipkg"
	"github.com/ehabterra/apispec/testdata/cli_action_cross_package/web"
)

// CmdWeb is gitea's shape: a package-level var holding the command, whose Action
// is a function from another package.
var CmdWeb = &clipkg.Command{Name: "web", Action: web.RunWeb}

func main() {
	srv := &web.Server{}
	app := &clipkg.App{Commands: []*clipkg.Command{
		CmdWeb,
		{Name: "admin", Action: srv.RunAdmin},
	}}
	_ = app.Run("web")
}
