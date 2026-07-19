package common

import (
	"encoding/json"
	"net/http"
)

type Response struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func RespondWithSuccess(w http.ResponseWriter, msg string, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(Response{Message: msg, Data: data})
}
