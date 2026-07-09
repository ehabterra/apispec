// Package main exercises a common Go pattern: every handler routes its
// response through an envelope helper that wraps the payload in a
// shared {message, data, code} struct whose `data` field is declared
// as interface{}. A shape seen in production codebases, faithfully reduced.
//
// Without field-provenance, every route's response would render as the
// same Envelope schema with `data: object` and the per-route payload
// type would be lost.
//
// With field-provenance:
//
//	/orders    → Envelope { message, code, data: $ref(Order) }
//	/customers → Envelope { message, code, data: $ref(Customer) }
//
// One source struct, two specialised schemas, one per route — each
// reflecting the concrete payload type the handler actually passes
// through common.RespondWithSuccess.
package main

import (
	"net/http"

	"testdata/wrapped_response/common"
	"testdata/wrapped_response/customers"
	"testdata/wrapped_response/orders"
	"testdata/wrapped_response/transactions"
)

func listOrders(w http.ResponseWriter, r *http.Request) {
	resp := orders.Order{ID: "o-1", Total: 42}
	common.RespondWithSuccess(w, "ok", resp, http.StatusOK)
}

func listCustomers(w http.ResponseWriter, r *http.Request) {
	resp := customers.Customer{ID: "c-1", Email: "c@example.com"}
	common.RespondWithSuccess(w, "ok", resp, http.StatusOK)
}

// listTransactions mirrors a production payment-handler list endpoint:
// the payload is a `var`-declared DTO whose `[]any` field is populated by
// appending, then passed by value to the envelope helper — not a composite
// literal. The specialiser must recover transactions.ListTransactionResponse
// for `data` and the component must be emitted.
func listTransactions(w http.ResponseWriter, r *http.Request) {
	var mappedResp transactions.ListTransactionResponse
	for i := 0; i < 3; i++ {
		mappedResp.Transactions = append(mappedResp.Transactions, i)
	}
	common.RespondWithSuccess(w, "ok", mappedResp, http.StatusOK)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/orders", listOrders)
	mux.HandleFunc("/customers", listCustomers)
	mux.HandleFunc("/transactions", listTransactions)
	_ = http.ListenAndServe(":8080", mux)
}
