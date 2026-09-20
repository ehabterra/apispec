// A serializer takes a value and returns bytes, so nothing about the call says
// where those bytes GO. The shipped default never has to ask: it anchors on the
// write sink and traces back through ResponseContext.BodyTransforms, so a
// marshal no sink reaches is simply never found (issue #195).
//
// A pattern the USER writes gets none of that. `^Marshal$` with typeFromArg
// matches every marshal in the handler's call graph, including the body of an
// outbound request — which on one real service documented a mail provider's
// payload struct, an ERP credential struct and context.Context as responses
// (issue #519).
//
// Both shapes are here, and they are identical up to where the bytes go.
package main

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Widget struct {
	ID string `json:"id"`
}

// Payload is a third-party provider's REQUEST body. It is never a response of
// this service.
type Payload struct {
	To string `json:"to"`
}

// send marshals an outbound request body. The bytes go to http.NewRequest.
func send(to string) error {
	b, _ := json.Marshal(Payload{To: to})
	req, _ := http.NewRequest(http.MethodPost, "https://provider.example/send", bytes.NewReader(b))
	_, err := http.DefaultClient.Do(req)
	return err
}

// createWidget calls the provider as a side effect and answers with its own
// type. The marshal it reaches is not its response.
func createWidget(w http.ResponseWriter, r *http.Request) {
	if err := send("a@b.c"); err != nil {
		http.Error(w, "upstream", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Widget{ID: "1"})
}

// getWidget marshals and writes the bytes itself — the legitimate shape, which
// must keep its type whether or not the serializer pattern is configured.
func getWidget(w http.ResponseWriter, r *http.Request) {
	b, _ := json.Marshal(Widget{ID: "1"})
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func main() {
	r := chi.NewRouter()
	r.Post("/widgets", createWidget)
	r.Get("/widgets/1", getWidget)
	_ = http.ListenAndServe(":8080", r)
}
