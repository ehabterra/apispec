package webctx

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

type Job struct {
	Name string `yaml:"name"`
}

// Workflow serializes jobs into its own buffer. Nothing it encodes reaches the
// response, so a handler that calls it documents none of it.
type Workflow struct {
	Raw []byte
}

func (w *Workflow) SetJob(id string, job *Job) error {
	m := map[string]*Job{id: job}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(m); err != nil {
		return err
	}
	w.Raw = buf.Bytes()
	return nil
}
