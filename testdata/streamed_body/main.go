// A handler that STREAMS has nothing for response detection to see: detection
// recognises a value being encoded, and these write bytes. So the operation
// documented no success at all — only whatever error branch used a recognised
// helper, leaving a CSV export claiming it can only fail (issue #517).
//
// The media type comes from what the handler DECLARES rather than from which
// writer it used. Enumerating writers does not scale — csv, gzip, bufio,
// io.Copy, fmt.Fprint, ServeContent and still not excelize or archive/zip —
// and it can only guess the type, where `Content-Type` states it.
package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

func csvExport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"a", "b"}); err != nil {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	cw.Flush()
}

func download(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open("/tmp/x.pdf")
	if err != nil {
		http.Error(w, "boom", http.StatusInternalServerError)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = io.Copy(w, f)
}

func plain(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "ok %d", 1)
}

// Negative: a csv writer over a BUFFER that never reaches the wire.
func internalOnly(w http.ResponseWriter, r *http.Request) {
	f, _ := os.Create("/tmp/out.csv")
	defer f.Close()
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{"x"})
	cw.Flush()
	w.WriteHeader(http.StatusNoContent)
}

// mimeCSV is how a real project names its media types — in one place, not
// inlined at each handler. Reading only a bare literal missed every such
// project, which is most of them.
const mimeCSV = "text/csv"

// The media type named by a CONSTANT.
func exportConst(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", mimeCSV)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"a"})
	cw.Flush()
}

// The declaration made inside a shared helper, beside the other headers a
// download needs.
func setCSVHeaders(w http.ResponseWriter, name string) {
	w.Header().Set("Content-Disposition", "attachment; filename="+name)
	w.Header().Set("Content-Type", mimeCSV)
}

func exportHelper(w http.ResponseWriter, r *http.Request) {
	setCSVHeaders(w, "audit.csv")
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"a"})
	cw.Flush()
}

func main() {
	r := chi.NewRouter()
	r.Get("/export.csv", csvExport)
	r.Get("/file.pdf", download)
	r.Get("/plain", plain)
	r.Get("/internal", internalOnly)
	r.Get("/const.csv", exportConst)
	r.Get("/helper.csv", exportHelper)
	_ = http.ListenAndServe(":8080", r)
}
