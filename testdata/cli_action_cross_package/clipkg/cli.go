// Package clipkg stands in for urfave/cli: the command type whose Action field
// holds the function the dispatcher calls back.
package clipkg

// Command mirrors cli.Command.
type Command struct {
	Name   string
	Action func() error
}

// App mirrors cli.App.
type App struct {
	Commands []*Command
}

// Run dispatches to the named command's Action — the dynamic hop that no static
// call edge crosses.
func (a *App) Run(name string) error {
	for _, c := range a.Commands {
		if c.Name == name {
			return c.Action()
		}
	}
	return nil
}
