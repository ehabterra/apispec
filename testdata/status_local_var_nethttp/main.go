package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func writeHeader(w http.ResponseWriter, _ *http.Request) {
	code := http.StatusConflict
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Item{})
}

func httpError(w http.ResponseWriter, _ *http.Request) {
	code := http.StatusTeapot
	http.Error(w, "no", code)
}

func main() {
	http.HandleFunc("GET /write-header", writeHeader)
	http.HandleFunc("GET /http-error", httpError)
	_ = http.ListenAndServe(":8080", nil)
}
