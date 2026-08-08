// Fixture: the net/http response catch-all must be anchored on the writer being
// somewhere in the call (issue #302).
//
// The preset matches any call named JSON/String/XML/YAML/ProtoBuf/Data/File/
// Redirect and documents its SECOND argument as the response body. Unanchored,
// that included calls in any reached package that merely share a name — and
// APISpec shipped it that way, so this was a default-config defect rather than a
// misconfiguration.
//
// TWO ROUTES, because the two halves must not be able to mask each other:
//
//	/widget  reaches report.Data(ctx, payload) — an analytics helper that never
//	         sees a writer. Its payload must NOT be documented. The real body
//	         goes through writeJSON, whose name the catch-all does not match, so
//	         nothing competes with the impostor for the slot: if the two shared a
//	         route the lexicographic tie-break could drop the impostor on its own
//	         and the fixture would pass without the anchor doing anything.
//	/gadget  is rendered by Data(w, v) — the catch-all's own shape, handed the
//	         writer. It must still resolve, or "anchoring" would just mean the
//	         catch-all stopped working.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/ehabterra/apispec/testdata/http_catchall_anchor/report"
)

type Widget struct {
	ID string `json:"id"`
}

type Gadget struct {
	Name string `json:"name"`
}

// writeJSON is deliberately NOT one of the catch-all's names; its inner Encode
// is matched by the properly anchored encoder pattern.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Data has the catch-all's shape and IS handed the writer.
func Data(w http.ResponseWriter, v any) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

func getWidget(w http.ResponseWriter, r *http.Request) {
	_ = report.Data(r.Context(), report.Payload{Rows: 1}) // analytics, not a response
	writeJSON(w, http.StatusOK, Widget{ID: "1"})
}

func getGadget(w http.ResponseWriter, r *http.Request) {
	Data(w, Gadget{Name: "g"})
}

func main() {
	http.HandleFunc("/widget", getWidget)
	http.HandleFunc("/gadget", getGadget)
	_ = http.ListenAndServe(":8080", nil)
}
