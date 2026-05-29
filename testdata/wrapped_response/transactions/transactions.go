// Package transactions reproduces the lmd-core
// dtos.ListTransactionResponse shape: a named response struct whose
// only field is a slice of `any`. The payload is interesting because
// the handler doesn't pass a composite literal — it declares a local
// `var`, appends to its slice field, then hands the value to the
// envelope helper. The wrapper specialiser must still recover this
// concrete type for the `data` field AND ensure the referenced
// component is registered (regression for the dangling-$ref bug where
// `data: $ref(ListTransactionResponse)` pointed at a component that was
// never emitted).
package transactions

// ListTransactionResponse mirrors the real DTO: `Transactions []any`.
type ListTransactionResponse struct {
	Transactions []any `json:"transactions"`
}
