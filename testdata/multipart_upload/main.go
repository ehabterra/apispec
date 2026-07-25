// Fixture: multipart/form-data request bodies (issue #207). Nothing in a Go
// signature says a body is multipart — the same r.FormValue reads serve both
// media types — so the distinguishing fact is *how* the handler reads it:
// ParseMultipartForm/MultipartReader parse a multipart body, and FormFile reads
// a file part from one. Without that, a file upload documented itself as
// application/x-www-form-urlencoded with the file part missing entirely.
package main

import (
	"encoding/json"
	"net/http"
)

type UploadResult struct {
	ID string `json:"id"`
}

// upload is the issue's shape: a parsed multipart form, one file part read by
// name, and a text field alongside it.
func upload(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(32 << 20)
	file, header, err := r.FormFile("avatar")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	_ = header.Filename
	title := r.FormValue("title")
	_ = title
	_ = json.NewEncoder(w).Encode(UploadResult{ID: "1"})
}

// uploadNoParse omits ParseMultipartForm — FormFile alone is enough, since no
// framework offers it for anything but a multipart body.
func uploadNoParse(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("document")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer file.Close()
	_ = json.NewEncoder(w).Encode(UploadResult{ID: "2"})
}

// parseOnly reads no part by name: the multipart marker is the only evidence,
// and the text field must still be documented under multipart/form-data rather
// than urlencoded.
func parseOnly(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(1 << 20)
	_ = r.FormValue("caption")
	_ = json.NewEncoder(w).Encode(UploadResult{ID: "3"})
}

// plainForm is the control: FormValue with no multipart evidence anywhere stays
// application/x-www-form-urlencoded (issue #171 behaviour, which must not drift).
func plainForm(w http.ResponseWriter, r *http.Request) {
	_ = r.FormValue("name")
	_ = json.NewEncoder(w).Encode(UploadResult{ID: "4"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload", upload)
	mux.HandleFunc("POST /documents", uploadNoParse)
	mux.HandleFunc("POST /captions", parseOnly)
	mux.HandleFunc("POST /plain", plainForm)
	_ = http.ListenAndServe(":8080", mux)
}
