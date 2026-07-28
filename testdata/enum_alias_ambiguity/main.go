// Fixture: two string ALIASES in one package, each with its own constants.
//
// `type ModelType = string` is an alias, not a new type, so a field declared
// `ModelType` has the type `string` — exactly like a field declared `ApiFormat`.
// Both constant groups are therefore candidates for the same field's enum, and
// nothing in the types can separate them.
//
// Seen on a real project (photoprism, internal/ai/vision): the spec documented
// `enum: [images, ollama, url, vision]` on some runs and
// `enum: [caption, face, labels, nsfw]` on others — the same code, a different
// answer per run, and wrong in at least one of them.
package main

import (
	"encoding/json"
	"net/http"
)

type ApiFormat = string

const (
	ApiFormatUrl    ApiFormat = "url"
	ApiFormatImages ApiFormat = "images"
	ApiFormatVision ApiFormat = "vision"
	ApiFormatOllama ApiFormat = "ollama"
)

type ModelType = string

const (
	ModelTypeLabels  ModelType = "labels"
	ModelTypeNsfw    ModelType = "nsfw"
	ModelTypeFace    ModelType = "face"
	ModelTypeCaption ModelType = "caption"
)

// Status is a real named type — a distinct type, so its constants are the only
// candidates for a Status field and the enum is knowable.
type Status string

const (
	StatusActive  Status = "active"
	StatusPaused  Status = "paused"
	StatusRetired Status = "retired"
)

type Model struct {
	Name   string    `json:"name"`
	Type   ModelType `json:"type"`
	Status Status    `json:"status"`
}

func getModel(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Model{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /models", getModel)
	_ = http.ListenAndServe(":8080", mux)
}
