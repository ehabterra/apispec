package items

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Repo struct{}

func New() *Repo { return &Repo{} }

// List returns []Item — the slice type the enclosing handler's `out`
// variable is bound to, and the type the response body schema must
// resolve to after parameter-tracing through the writeJSON helper.
func (r *Repo) List(filter string) ([]Item, error) {
	return nil, nil
}
