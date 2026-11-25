package webctx

import "net/http"

// ResponseWriter is the project's own writer interface; Base writes through
// it, never through http.ResponseWriter directly.
type ResponseWriter interface {
	http.ResponseWriter
	WrittenStatus() int
}

// Response wraps the server's writer and records the status it sent.
type Response struct {
	http.ResponseWriter
	status int
}

func (r *Response) Write(bs []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(bs)
}

func (r *Response) WriteHeader(statusCode int) {
	if r.status == 0 {
		r.status = statusCode
		r.ResponseWriter.WriteHeader(statusCode)
	}
}

func (r *Response) WrittenStatus() int {
	return r.status
}

func WrapResponseWriter(w http.ResponseWriter) *Response {
	if v, ok := w.(*Response); ok {
		return v
	}
	return &Response{ResponseWriter: w}
}
