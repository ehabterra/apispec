package ws

import (
	"net/http"
	"runtime"

	"another-chi-router/models"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/gorilla/websocket"
)

// Handler handles user routes
type Handler struct {
	// In a real application, you would inject dependencies like:
	// userService user.Service
}

// New creates a new user handler
func New() *Handler {
	return &Handler{}
}

// Routes returns the user routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.HandleFunc("/", h.websocket)
	return r
}

// websocket handles websocket connections
func (h *Handler) websocket(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "invalid request",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// upgrade connection
	upgrader := websocket.Upgrader{}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		render.Status(r, http.StatusInternalServerError)
		render.JSON(w, r, models.ErrorResponse{
			Error: err.Error(),
			Code:  http.StatusInternalServerError,
		})
		return
	}
	runtime.KeepAlive(conn)
}
