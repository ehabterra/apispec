// Package storage is reached through an INLINE interface field, so the receiver
// type as written has no name of its own.
package storage

type Asset struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

type S3 struct{}

func (s *S3) Describe() Asset { return Asset{} }
