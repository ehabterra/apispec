package broken

// Row is the type the route actually returns. This package does not COMPILE —
// the reference below is undefined — so the loader reports errors for it and
// apispec skips it, exactly as it skips a package that is mid-edit. Its types
// are therefore not recorded, which is the state that made a real service fill
// five components from other packages (issue #447).
type Row struct {
	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`
}

var _ = thisSymbolDoesNotExist
