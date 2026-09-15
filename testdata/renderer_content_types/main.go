package main

// Two encoders, one call name. encoding/xml and encoding/json both spell it
// Encode, so only the receiver says which format reaches the client.

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"net/http"
)

type Item struct {
	ID   int    `json:"id" xml:"id"`
	Name string `json:"name" xml:"name"`
}

func getItemXML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(Item{})
}

// The same encode with NO Content-Type header. Go then sniffs the payload and
// sends text/plain; charset=utf-8 (the sniffer only says text/xml when the
// document starts with the `<?xml` declaration, which the encoder does not
// write). The encoder still says the body is XML, and that is what is
// documented today — reading the header, so it can win, is #354's remaining
// step.
func getItemXMLNoHeader(w http.ResponseWriter, r *http.Request) {
	_ = xml.NewEncoder(w).Encode(Item{})
}

func getItemJSON(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

func getItemBoth(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/xml" {
		_ = xml.NewEncoder(w).Encode(Item{})
		return
	}
	_ = json.NewEncoder(w).Encode(Item{})
}

// legacyCode is a NAMED UNTYPED CONSTANT. Encoding one is unusual but legal,
// and it is the case a gate in #484 wrongly rejected: that gate excluded untyped
// bodies from media-type composition to suppress a mis-resolved status argument,
// so a genuine constant payload lost one of its two representations. #485 fixed
// the mis-resolution at its source, and this keeps the recovered case honest.
const legacyCode = 7

func legacyBoth(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Accept") == "application/xml" {
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(legacyCode)
		return
	}
	_ = json.NewEncoder(w).Encode(legacyCode)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/xml", getItemXML)
	mux.HandleFunc("GET /items/json", getItemJSON)
	mux.HandleFunc("GET /items/both", getItemBoth)
	mux.HandleFunc("GET /items/away", encodeAway)
	mux.HandleFunc("GET /items/xml-no-header", getItemXMLNoHeader)
	mux.HandleFunc("GET /items/legacy", legacyBoth)
	_ = http.ListenAndServe(":8080", mux)
}

// encodeAway writes YAML-ish output to a buffer that never reaches the
// response — the shape a workflow serializer or a config dumper has. It must
// not be documented as this endpoint's body (issues #354, #471).
func encodeAway(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	enc := xml.NewEncoder(&buf)
	_ = enc.Encode(Item{})
	_ = json.NewEncoder(w).Encode(Item{})
}
