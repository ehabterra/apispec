// Package main is a robustness stress fixture for dense call graphs.
//
// Historical failure: issue #20 (hang on scan — an Echo+swaggo project with 23
// endpoints whose tracker tree expansion was exponential in the dense call
// graph). This fixture reproduces that shape at a realistic scale: 25 HTTP
// handlers fan into a shared layer of 10 services, which fan into 8 repos,
// which fan into 6 leaf helpers, every layer with high fan-in over the shared
// DTO. The graph is dense in node/edge count but bounded in depth, so with the
// tracker's traversal limits in place generation completes quickly. The
// accompanying TestTestdata_DenseGraphBounded asserts it finishes within a wall-
// clock budget; an unbounded-traversal regression trips that timeout.
package main

import (
	"encoding/json"
	"net/http"
)

// Payload is the shared DTO threaded through every layer of the graph.
type Payload struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func leaf0(p Payload) Payload {
	p.Kind = "leaf0"
	return p
}

func leaf1(p Payload) Payload {
	p.Kind = "leaf1"
	return p
}

func leaf2(p Payload) Payload {
	p.Kind = "leaf2"
	return p
}

func leaf3(p Payload) Payload {
	p.Kind = "leaf3"
	return p
}

func leaf4(p Payload) Payload {
	p.Kind = "leaf4"
	return p
}

func leaf5(p Payload) Payload {
	p.Kind = "leaf5"
	return p
}

func repo0(p Payload) Payload {
	p = leaf0(p)
	p = leaf1(p)
	p = leaf2(p)
	return p
}

func repo1(p Payload) Payload {
	p = leaf1(p)
	p = leaf2(p)
	p = leaf3(p)
	return p
}

func repo2(p Payload) Payload {
	p = leaf2(p)
	p = leaf3(p)
	p = leaf4(p)
	return p
}

func repo3(p Payload) Payload {
	p = leaf3(p)
	p = leaf4(p)
	p = leaf5(p)
	return p
}

func repo4(p Payload) Payload {
	p = leaf4(p)
	p = leaf5(p)
	p = leaf0(p)
	return p
}

func repo5(p Payload) Payload {
	p = leaf5(p)
	p = leaf0(p)
	p = leaf1(p)
	return p
}

func repo6(p Payload) Payload {
	p = leaf0(p)
	p = leaf1(p)
	p = leaf2(p)
	return p
}

func repo7(p Payload) Payload {
	p = leaf1(p)
	p = leaf2(p)
	p = leaf3(p)
	return p
}

func service0(p Payload) Payload {
	p = repo0(p)
	p = repo1(p)
	p = repo2(p)
	p = repo3(p)
	return p
}

func service1(p Payload) Payload {
	p = repo1(p)
	p = repo2(p)
	p = repo3(p)
	p = repo4(p)
	return p
}

func service2(p Payload) Payload {
	p = repo2(p)
	p = repo3(p)
	p = repo4(p)
	p = repo5(p)
	return p
}

func service3(p Payload) Payload {
	p = repo3(p)
	p = repo4(p)
	p = repo5(p)
	p = repo6(p)
	return p
}

func service4(p Payload) Payload {
	p = repo4(p)
	p = repo5(p)
	p = repo6(p)
	p = repo7(p)
	return p
}

func service5(p Payload) Payload {
	p = repo5(p)
	p = repo6(p)
	p = repo7(p)
	p = repo0(p)
	return p
}

func service6(p Payload) Payload {
	p = repo6(p)
	p = repo7(p)
	p = repo0(p)
	p = repo1(p)
	return p
}

func service7(p Payload) Payload {
	p = repo7(p)
	p = repo0(p)
	p = repo1(p)
	p = repo2(p)
	return p
}

func service8(p Payload) Payload {
	p = repo0(p)
	p = repo1(p)
	p = repo2(p)
	p = repo3(p)
	return p
}

func service9(p Payload) Payload {
	p = repo1(p)
	p = repo2(p)
	p = repo3(p)
	p = repo4(p)
	return p
}

func handler0(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service0(req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler1(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler2(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler3(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler4(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	req = service7(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler5(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service5(req)
	req = service6(req)
	req = service7(req)
	req = service8(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler6(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service6(req)
	req = service7(req)
	req = service8(req)
	req = service9(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler7(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service7(req)
	req = service8(req)
	req = service9(req)
	req = service0(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler8(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service8(req)
	req = service9(req)
	req = service0(req)
	req = service1(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler9(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service9(req)
	req = service0(req)
	req = service1(req)
	req = service2(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler10(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service0(req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler11(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler12(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler13(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler14(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	req = service7(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler15(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service5(req)
	req = service6(req)
	req = service7(req)
	req = service8(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler16(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service6(req)
	req = service7(req)
	req = service8(req)
	req = service9(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler17(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service7(req)
	req = service8(req)
	req = service9(req)
	req = service0(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler18(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service8(req)
	req = service9(req)
	req = service0(req)
	req = service1(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler19(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service9(req)
	req = service0(req)
	req = service1(req)
	req = service2(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler20(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service0(req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler21(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service1(req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler22(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service2(req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler23(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service3(req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	_ = json.NewEncoder(w).Encode(req)
}

func handler24(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	req = service4(req)
	req = service5(req)
	req = service6(req)
	req = service7(req)
	_ = json.NewEncoder(w).Encode(req)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/route0", handler0)
	mux.HandleFunc("/route1", handler1)
	mux.HandleFunc("/route2", handler2)
	mux.HandleFunc("/route3", handler3)
	mux.HandleFunc("/route4", handler4)
	mux.HandleFunc("/route5", handler5)
	mux.HandleFunc("/route6", handler6)
	mux.HandleFunc("/route7", handler7)
	mux.HandleFunc("/route8", handler8)
	mux.HandleFunc("/route9", handler9)
	mux.HandleFunc("/route10", handler10)
	mux.HandleFunc("/route11", handler11)
	mux.HandleFunc("/route12", handler12)
	mux.HandleFunc("/route13", handler13)
	mux.HandleFunc("/route14", handler14)
	mux.HandleFunc("/route15", handler15)
	mux.HandleFunc("/route16", handler16)
	mux.HandleFunc("/route17", handler17)
	mux.HandleFunc("/route18", handler18)
	mux.HandleFunc("/route19", handler19)
	mux.HandleFunc("/route20", handler20)
	mux.HandleFunc("/route21", handler21)
	mux.HandleFunc("/route22", handler22)
	mux.HandleFunc("/route23", handler23)
	mux.HandleFunc("/route24", handler24)
	_ = http.ListenAndServe(":8080", mux)
}
