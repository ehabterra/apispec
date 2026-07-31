// Package dtos holds one request type per route, so a route documenting another
// route's type is visible.
package dtos

type AlphaRequest struct {
	CartID string `json:"cart_id"`
	Alpha  string `json:"alpha"`
}

type BravoRequest struct {
	CartID string `json:"cart_id"`
	Bravo  string `json:"bravo"`
}

type CharlieRequest struct {
	CartID  string `json:"cart_id"`
	Charlie string `json:"charlie"`
}

type DeltaRequest struct {
	CartID string `json:"cart_id"`
	Delta  string `json:"delta"`
}

type EchoRequest struct {
	CartID string `json:"cart_id"`
	Echo   string `json:"echo"`
}

type FoxtrotRequest struct {
	CartID  string `json:"cart_id"`
	Foxtrot string `json:"foxtrot"`
}

// Estimate is the shared field type at the centre of the defect: one route
// WRITES it from its own request DTO, another route READS it in its response
// converter.
type Estimate struct {
	Minutes int `json:"minutes"`
}
