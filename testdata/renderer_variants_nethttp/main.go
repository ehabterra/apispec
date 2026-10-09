package main

import (
	"bytes"
	"net/http"
	"os"
	"time"
)

func serveFile(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "a.pdf") }

func serveFileFS(w http.ResponseWriter, r *http.Request) {
	http.ServeFileFS(w, r, os.DirFS("."), "a.pdf")
}

func serveContent(w http.ResponseWriter, r *http.Request) {
	http.ServeContent(w, r, "a.pdf", time.Time{}, bytes.NewReader(nil))
}

func redirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/x", http.StatusFound)
}

func main() {
	http.HandleFunc("GET /serve-file", serveFile)
	http.HandleFunc("GET /serve-file-fs", serveFileFS)
	http.HandleFunc("GET /serve-content", serveContent)
	http.HandleFunc("GET /redirect", redirect)
	_ = http.ListenAndServe(":8080", nil)
}
