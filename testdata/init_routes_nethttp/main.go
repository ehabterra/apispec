package main

import (
	"encoding/json"
	"net/http"

	_ "github.com/ehabterra/apispec/testdata/init_routes_nethttp/plugins"
)

type A struct {
	A int `json:"a"`
}

type B struct {
	B int `json:"b"`
}

type C struct {
	C int `json:"c"`
}

type D struct {
	D int `json:"d"`
}

func a(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(A{}) }
func b(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(B{}) }
func c(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(C{}) }
func d(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(D{}) }

// A package may declare several init functions, even in one file.
func init() { http.HandleFunc("/from-init", b) }

func init() { http.HandleFunc("/from-second-init", c) }

func register(path string, h http.HandlerFunc) bool {
	http.HandleFunc(path, h)
	return true
}

// A package-level var initializer runs before main too.
var _ = register("/from-var", d)

func main() {
	http.HandleFunc("/from-main", a)
	_ = http.ListenAndServe(":8080", nil)
}
