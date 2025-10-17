package handler

import (
	"net/http"

	"another-chi-router/handler/ws"

	"github.com/go-chi/chi/v5"
)

type WSHandler struct{}

func NewWebsocket() *WSHandler {
	return &WSHandler{}
}

func (h *WSHandler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Mount("/websocket", ws.New().Routes())
	return r
}
