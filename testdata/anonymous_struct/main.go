// Package main exercises the OpenAPI generator's handling of anonymous
// (inline) structs used as request and response bodies.
//
// Anonymous struct request bodies cannot be referenced via $ref because
// they have no name. APISpec must either:
//
//   - synthesize a deterministic schema name (e.g. derived from operation
//     id) and emit it under components/schemas, or
//
//   - inline the schema directly on the requestBody.content.*.schema node.
//
// In either case the generated spec must accurately describe the anonymous
// struct's fields, including embedded named types like itemReq, which
// SHOULD be referenced via $ref.
//
// Routes:
//
//   - POST /orders        — anonymous struct request: { items: []itemReq }
//   - POST /bulk-update   — anonymous struct with multiple fields
//   - POST /tags          — anonymous struct of primitives
//   - GET  /summary       — anonymous struct response
package main

import (
	stdhttp "net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
)

// itemReq is a named type referenced from an anonymous struct's field.
// It MUST appear in components/schemas because it is reachable via the
// /orders request body.
type itemReq struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

// updateOp is referenced from /bulk-update's anonymous struct.
type updateOp struct {
	Path  string `json:"path"`
	Value string `json:"value"`
}

// summaryStat is referenced from /summary's anonymous response struct.
type summaryStat struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func main() {
	r := chi.NewRouter()
	r.Post("/orders", createOrder)
	r.Post("/bulk-update", bulkUpdate)
	r.Post("/tags", addTags)
	r.Get("/summary", getSummary)
	_ = stdhttp.ListenAndServe(":8080", r)
}

// createOrder decodes an anonymous struct that wraps a slice of a named
// type. The generated spec MUST expose itemReq under components/schemas
// and the anonymous wrapper must describe { items: []$ref(itemReq) }.
func createOrder(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req struct {
		Items []itemReq `json:"items"`
	}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		stdhttp.Error(w, "invalid JSON", stdhttp.StatusBadRequest)
		return
	}
	render.Status(r, stdhttp.StatusCreated)
	render.JSON(w, r, map[string]any{"accepted": len(req.Items)})
}

// bulkUpdate decodes an anonymous struct with multiple heterogeneous
// fields, including a primitive, a slice of named type, and a nested
// anonymous struct.
func bulkUpdate(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req struct {
		Reason string     `json:"reason"`
		Ops    []updateOp `json:"ops"`
		Meta   struct {
			Source string `json:"source"`
			DryRun bool   `json:"dry_run"`
		} `json:"meta"`
	}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		stdhttp.Error(w, "invalid JSON", stdhttp.StatusBadRequest)
		return
	}
	render.JSON(w, r, map[string]any{"applied": len(req.Ops)})
}

// addTags decodes an anonymous struct of primitives only. No named type
// is reachable through it, so nothing extra should appear under
// components/schemas because of this route.
func addTags(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		stdhttp.Error(w, "invalid JSON", stdhttp.StatusBadRequest)
		return
	}
	render.Status(r, stdhttp.StatusNoContent)
	_ = req
}

// getSummary returns an anonymous struct as its response body. The
// generated spec MUST describe the response shape and reference
// summaryStat via $ref.
func getSummary(w stdhttp.ResponseWriter, r *stdhttp.Request) {
	resp := struct {
		Total int           `json:"total"`
		Stats []summaryStat `json:"stats"`
	}{
		Total: 2,
		Stats: []summaryStat{
			{Label: "open", Count: 1},
			{Label: "closed", Count: 1},
		},
	}
	render.JSON(w, r, resp)
}
