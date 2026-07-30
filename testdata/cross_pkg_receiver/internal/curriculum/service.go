// Package curriculum holds a service whose methods a handler calls through a
// struct field, from another package.
package curriculum

type Subject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Service struct{}

func (s *Service) ListSubjects() []Subject {
	return []Subject{}
}
