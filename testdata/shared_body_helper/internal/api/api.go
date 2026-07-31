// Package api holds the handlers and the shared domain/response mapping.
package api

import (
	"net/http"

	"example.com/sharedbodyhelper/internal/common"
	"example.com/sharedbodyhelper/internal/dtos"
	"example.com/sharedbodyhelper/internal/httpx"
)

// Cart is the one domain type every route works on.
type Cart struct {
	ID       string
	Estimate dtos.Estimate
}

// View is the response shape.
type View struct {
	ID       string        `json:"id"`
	Estimate dtos.Estimate `json:"estimate"`
}

type Service struct{}

func (s *Service) Load(id string) *Cart { return &Cart{ID: id} }

// applyEstimate WRITES Cart.Estimate from a request DTO. It is what makes the
// producer of `cart.Estimate` a SIBLING route's handler.
func applyEstimate(c *Cart, req *dtos.BravoRequest) {
	c.Estimate = dtos.Estimate{Minutes: len(req.Bravo)}
}

// estimateView READS Cart.Estimate. Passing the field as an argument is what
// makes the tracker resolve "who produced this value" — and answer with the
// handler that wrote it, dragging that route's whole body in behind it.
func estimateView(e dtos.Estimate) dtos.Estimate { return e }

// ToView is the shared response converter every handler calls.
func ToView(c *Cart) View {
	v := View{ID: c.ID}
	v.Estimate = estimateView(c.Estimate)
	return v
}

func PostAlpha(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.AlphaRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}

func PostBravo(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.BravoRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		applyEstimate(cart, &req)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}

func PostCharlie(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.CharlieRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}

func PostDelta(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.DeltaRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}

func PostEcho(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.EchoRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}

func PostFoxtrot(svc *Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := httpx.DecodeJSON[dtos.FoxtrotRequest](r)
		if err != nil {
			common.RespondWithError(w, http.StatusBadRequest)
			return
		}
		cart := svc.Load(req.CartID)
		common.RespondWithSuccess(w, "true", ToView(cart), http.StatusOK)
	}
}
