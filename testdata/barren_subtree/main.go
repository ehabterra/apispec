// Package main exercises the barren-subtree prune (issue #318): a handler whose
// real work — decoding a body, writing a response — sits behind a large subtree
// of calls that match no pattern at all.
//
// The utility layer below is deliberately dense and mutually recursive, so the
// per-path unfolding reaches it along many distinct paths. None of it can
// contribute to the spec, so the expansion is free to skip it; what this fixture
// guards is that skipping it does not take the route, its request body or its
// response with it.
package main

import (
	"encoding/json"
	"net/http"
)

type CreateOrderRequest struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type Order struct {
	ID    string `json:"id"`
	SKU   string `json:"sku"`
	Total int    `json:"total"`
}

// The barren layer. Nothing here is a route, a body, a param or a response —
// it is the shape that makes a real project's tree explode.

func normalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return trim(pad(s))
}

func pad(s string) string {
	if len(s) > 4 {
		return s
	}
	return pad(s + "_")
}

func trim(s string) string {
	if len(s) < 2 {
		return s
	}
	return classify(s)
}

func classify(s string) string {
	switch {
	case len(s)%3 == 0:
		return score(s)
	case len(s)%3 == 1:
		return weigh(s)
	default:
		return s
	}
}

func score(s string) string {
	if len(s) > 32 {
		return s
	}
	return weigh(s + "s")
}

func weigh(s string) string {
	if len(s) > 24 {
		return s
	}
	return normalize(s[:len(s)-1])
}

func total(q int) int {
	if q <= 0 {
		return 0
	}
	return q*price(q) + surcharge(q)
}

func price(q int) int {
	if q > 10 {
		return 5
	}
	return 10 - q
}

func surcharge(q int) int {
	if q < 2 {
		return 0
	}
	return price(q) / 2
}

// createOrder is the route. Its body and response must survive the prune, and
// every barren helper above is reachable from it.
func createOrder(w http.ResponseWriter, r *http.Request) {
	var req CreateOrderRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	order := Order{
		ID:    normalize(req.SKU),
		SKU:   normalize(req.SKU),
		Total: total(req.Quantity),
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(order)
}

// listOrders shares the same barren helpers, so they are reached along more than
// one route's paths — the diamond that the per-path unfolding duplicates.
func listOrders(w http.ResponseWriter, r *http.Request) {
	_ = normalize("seed")
	_ = json.NewEncoder(w).Encode([]Order{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", createOrder)
	mux.HandleFunc("GET /orders", listOrders)
	_ = http.ListenAndServe(":8080", mux)
}
