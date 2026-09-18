// Package main covers an encode whose destination is a BUFFER (issue #471).
//
// The write-destination gate resolves an encode's destination and drops the
// encode when that destination provably is not the response writer. It sees one
// hop, so these two are indistinguishable to it — both encode into a
// *bytes.Buffer:
//
//	encoder.Encode(v); buf.WriteTo(w)   // the buffer IS the response
//	enc.Encode(v); return buf.Bytes()   // the buffer is internal
//
// Keeping both documents a response the endpoint never sends; dropping both
// loses a real one. Taken from gitea, where the first shape is the sitemap
// handlers — 18 correct application/xml responses that the gate currently drops.
package main

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
)

// Sitemap is the body the buffered encode really sends.
type Sitemap struct {
	URLs []string `xml:"url"`
}

// Internal is encoded into a buffer that never reaches the wire.
type Internal struct {
	Secret string `xml:"secret"`
}

// writeToSink encodes into a buffer and then flushes the buffer to the
// response writer. The body IS documented.
func writeToSink(w http.ResponseWriter, r *http.Request) {
	buf := bytes.NewBufferString(xml.Header)
	if err := xml.NewEncoder(buf).Encode(Sitemap{}); err != nil {
		return
	}
	_, _ = buf.WriteTo(w)
}

// copySink is the same thing spelled with io.Copy.
func copySink(w http.ResponseWriter, r *http.Request) {
	buf := &bytes.Buffer{}
	if err := xml.NewEncoder(buf).Encode(Sitemap{}); err != nil {
		return
	}
	_, _ = io.Copy(w, buf)
}

// discarded encodes into a buffer that is never written anywhere. Nothing about
// it may be documented: the endpoint sends no body at all.
func discarded(w http.ResponseWriter, r *http.Request) {
	var buf bytes.Buffer
	if err := xml.NewEncoder(&buf).Encode(Internal{}); err != nil {
		return
	}
	_ = buf.Len()
	w.WriteHeader(http.StatusNoContent)
}

// varEncoder is gitea's actual spelling: the encoder is assigned to a variable
// before Encode is called on it, so the destination cannot be read from a chain
// parent. Same buffer, same flush.
func varEncoder(w http.ResponseWriter, r *http.Request) {
	buf := bytes.NewBufferString(xml.Header)
	encoder := xml.NewEncoder(buf)
	if err := encoder.Encode(Sitemap{}); err != nil {
		return
	}
	_, _ = buf.WriteTo(w)
}

// WriteTo is gitea's real shape: the encode and the flush live in a METHOD on
// the payload type, whose writer is an io.Writer PARAMETER. The handler hands it
// the response writer.
func (s Sitemap) WriteTo(w io.Writer) (int64, error) {
	buf := bytes.NewBufferString(xml.Header)
	encoder := xml.NewEncoder(buf)
	if err := encoder.Encode(s); err != nil {
		return 0, err
	}
	return buf.WriteTo(w)
}

// viaHelper calls that method with the response writer.
func viaHelper(w http.ResponseWriter, r *http.Request) {
	m := Sitemap{}
	if _, err := m.WriteTo(w); err != nil {
		return
	}
}

// writeSitemap is the same work as the method above, but a plain function —
// to tell "one hop deeper" apart from "a method on the payload type".
func writeSitemap(w io.Writer, s Sitemap) error {
	buf := bytes.NewBufferString(xml.Header)
	encoder := xml.NewEncoder(buf)
	if err := encoder.Encode(s); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}

// viaFunc calls that plain helper with the response writer.
func viaFunc(w http.ResponseWriter, r *http.Request) {
	if err := writeSitemap(w, Sitemap{}); err != nil {
		return
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sitemap", writeToSink)
	mux.HandleFunc("GET /copied", copySink)
	mux.HandleFunc("GET /discarded", discarded)
	mux.HandleFunc("GET /var-encoder", varEncoder)
	mux.HandleFunc("GET /via-helper", viaHelper)
	mux.HandleFunc("GET /via-func", viaFunc)
	_ = http.ListenAndServe(":8080", mux)
}
