// Fixture: bodyless status codes (204/304/1xx) must be emitted with no
// `content` block per the OpenAPI spec (issue #169, GAP §7.3). Handlers here
// return 204 No Content, 304 Not Modified, and 100 Continue — none may carry a
// response body. getWidget returns a real 200 body to prove that non-bodyless
// responses are unaffected, and deleteWidget deliberately writes a spurious
// body alongside its 204 to prove the body is stripped anyway.
package main

import (
	"encoding/json"
	"net/http"
)

type Widget struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// deleteWidget writes a 204 — and a stray body — to prove the body is dropped.
func deleteWidget(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent) // 204 — must have no content
	_ = json.NewEncoder(w).Encode(Widget{ID: "deleted"})
}

// checkWidget returns 304 Not Modified — bodyless.
func checkWidget(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotModified) // 304 — must have no content
}

// continueUpload returns 100 Continue — a 1xx informational code, bodyless.
func continueUpload(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusContinue) // 100 — must have no content
}

// getWidget returns a normal 200 body — unaffected by the bodyless rule.
func getWidget(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK) // 200 — keeps its content block
	_ = json.NewEncoder(w).Encode(Widget{ID: "1", Name: "gadget"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /widget/{id}", deleteWidget)
	mux.HandleFunc("HEAD /widget/{id}", checkWidget)
	mux.HandleFunc("POST /upload", continueUpload)
	mux.HandleFunc("GET /widget/{id}", getWidget)
	_ = http.ListenAndServe(":8080", mux)
}
