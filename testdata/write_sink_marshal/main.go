package main

import (
	"encoding/json"
	"net/http"
)

// Payload is the real response body of GET /marshal-write. It reaches the wire
// through the decoupled shape `b, _ := json.Marshal(v); w.Write(b)` — the sink
// is w.Write, the body type lives one transform hop back on json.Marshal's arg.
type Payload struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// marshalWriteHandler is the shape issue #195's write-sink model must resolve:
// the marshal result is stored in a local, then written. Today w.Write(b) only
// sees b's []byte type; the fix traces b's assignment back to json.Marshal(v)
// and recovers v's type (Payload).
func marshalWriteHandler(w http.ResponseWriter, r *http.Request) {
	v := Payload{Key: "k", Count: 1}
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// rawWriteHandler writes raw bytes with no marshal transform behind them. The
// write-sink trace must find NO json payload here and produce a response with
// no JSON schema — not a spurious body.
func rawWriteHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("pong"))
}

// Envelope is the real response body of GET /helper-write, produced by a helper
// that returns the marshal result. Exercises the param/return hop of the
// backward trace (challenge #2): the transform is one function-boundary away
// from the sink.
type Envelope struct {
	Data string `json:"data"`
}

func encodeEnvelope(e Envelope) []byte {
	b, _ := json.Marshal(e)
	return b
}

func helperWriteHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encodeEnvelope(Envelope{Data: "d"}))
}

// Handler exercises the method-handler shape: a plain method (not a closure)
// is absent from the file's functions and has no ParentFunction, so the write
// sink must resolve its local `b := json.Marshal(m)` via the method table.
type Handler struct{}

// Member is the real response body of GET /method-write.
type Member struct {
	Name string `json:"name"`
}

func (h *Handler) MethodWrite(w http.ResponseWriter, r *http.Request) {
	m := Member{Name: "n"}
	b, _ := json.Marshal(m)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func main() {
	h := &Handler{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /marshal-write", marshalWriteHandler)
	mux.HandleFunc("GET /raw-write", rawWriteHandler)
	mux.HandleFunc("GET /helper-write", helperWriteHandler)
	mux.HandleFunc("GET /method-write", h.MethodWrite)
	_ = http.ListenAndServe(":8080", mux)
}
