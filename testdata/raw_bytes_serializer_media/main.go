package main

import (
	"encoding/json"
	"encoding/xml"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

type Item struct {
	ID string `json:"id" xml:"id" yaml:"id"`
}

var doc = []byte("openapi: 3.1.0\n")

func yamlCharset(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(doc)
}

func yamlPlain(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(doc)
}

func xmlRaw(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte("<ok/>"))
}

func textRaw(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func pdfRaw(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(doc)
}

func jsonRaw(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"1"}`))
}

func xmlEncoded(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	_ = xml.NewEncoder(w).Encode(Item{ID: "1"})
}

func yamlEncoded(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	_ = yaml.NewEncoder(w).Encode(Item{ID: "1"})
}

func jsonEncoded(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Item{ID: "1"})
}

func main() {
	r := chi.NewRouter()
	r.Get("/yaml-charset", yamlCharset)
	r.Get("/yaml", yamlPlain)
	r.Get("/xml", xmlRaw)
	r.Get("/text", textRaw)
	r.Get("/pdf", pdfRaw)
	r.Get("/json-raw", jsonRaw)
	r.Get("/xml-encoded", xmlEncoded)
	r.Get("/yaml-encoded", yamlEncoded)
	r.Get("/json-encoded", jsonEncoded)
	_ = http.ListenAndServe(":8080", r)
}
