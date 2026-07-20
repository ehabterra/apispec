package main

import (
	"net/http"

	"downstream_client_not_response/common"
	"downstream_client_not_response/usecase"
)

type dto struct {
	Key string `json:"key"`
}

func handler(uc usecase.UseCase) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		key, err := uc.Get(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		common.RespondWithSuccess(w, "ok", dto{Key: key}, http.StatusOK)
	}
}

func main() {
	uc := usecase.New()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pk", handler(uc))
	_ = http.ListenAndServe(":8080", mux)
}
