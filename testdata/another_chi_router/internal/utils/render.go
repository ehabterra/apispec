package utils

import (
	"errors"
	"log"
	"net/http"

	"another-chi-router/models"

	"github.com/go-chi/render"
)

// ErrResponse renderer type for handling all sorts of errors.
type ErrResponse struct {
	Err            error  `json:"-" example:"status bad request"` // low-level runtime error
	HTTPStatusCode int    `json:"-" example:"400"`                // http response status code
	RequestID      string `json:"request_id,omitempty"`           // request id for tracing request
	StatusText     string `json:"status"`                         // user-level status message
	ErrorText      string `json:"error,omitempty"`                // application-level error message, for debugging
}

func ErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, models.ErrUserFault) {
		RenderError(w, r, ErrBadRequest(err))
		return
	}
	if errors.Is(err, models.ErrNotFound) {
		RenderError(w, r, ErrNotFound(err))
		return
	}
	RenderError(w, r, ErrInternalServerError(err))
}

func (e *ErrResponse) Render(_ http.ResponseWriter, r *http.Request) error {
	// e.RequestID = ctxval.GetRequestID(r.Context())
	render.Status(r, e.HTTPStatusCode)
	return nil
}

func RenderError(w http.ResponseWriter, r *http.Request, v render.Renderer) {
	rerr := render.Render(w, r, v)
	if rerr != nil {
		log.Println("failed render response", "error", rerr)
	}
}

func ErrBadRequest(err error) render.Renderer {
	text := "Bad request"
	if err != nil {
		text = err.Error()
	}
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusBadRequest,
		StatusText:     "Bad request",
		ErrorText:      text,
	}
}

func ErrInternalServerError(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusInternalServerError,
		StatusText:     "Internal server error",
		ErrorText:      err.Error(),
	}
}

func ErrNotFound(err error) render.Renderer {
	return &ErrResponse{
		Err:            err,
		HTTPStatusCode: http.StatusNotFound,
		StatusText:     "Not found",
		ErrorText:      err.Error(),
	}
}
