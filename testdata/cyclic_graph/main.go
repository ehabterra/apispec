// Package main is a regression fixture for issue #20's worst case: a dense,
// strongly-connected (cyclic) call graph. Each of the 14 web functions calls
// three others by modular index, forming many back-edges; 12 handlers enter the
// web at staggered points. This shape re-expands shared callees along
// exponentially many cycle paths — it hung the tracker indefinitely until the
// cumulative MaxNodesPerTree cap bounded total traversal work. The recursive
// calls are dead code guarded by an early return so the fixture still compiles
// and runs; only the static call graph matters to apispec.
package main

import (
	"encoding/json"
	"net/http"
)

type Payload struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func f0(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f1(p)
	p = f3(p)
	p = f7(p)
	return p
}

func f1(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f2(p)
	p = f4(p)
	p = f8(p)
	return p
}

func f2(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f3(p)
	p = f5(p)
	p = f9(p)
	return p
}

func f3(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f4(p)
	p = f6(p)
	p = f10(p)
	return p
}

func f4(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f5(p)
	p = f7(p)
	p = f11(p)
	return p
}

func f5(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f6(p)
	p = f8(p)
	p = f12(p)
	return p
}

func f6(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f7(p)
	p = f9(p)
	p = f13(p)
	return p
}

func f7(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f8(p)
	p = f10(p)
	p = f0(p)
	return p
}

func f8(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f9(p)
	p = f11(p)
	p = f1(p)
	return p
}

func f9(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f10(p)
	p = f12(p)
	p = f2(p)
	return p
}

func f10(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f11(p)
	p = f13(p)
	p = f3(p)
	return p
}

func f11(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f12(p)
	p = f0(p)
	p = f4(p)
	return p
}

func f12(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f13(p)
	p = f1(p)
	p = f5(p)
	return p
}

func f13(p Payload) Payload {
	if p.ID > 100 {
		return p
	}
	p.ID++
	p = f0(p)
	p = f2(p)
	p = f6(p)
	return p
}

func handler0(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f0(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler1(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f1(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler2(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f2(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler3(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f3(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler4(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f4(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler5(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f5(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler6(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f6(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler7(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f7(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler8(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f8(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler9(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f9(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler10(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f10(req)
	_ = json.NewEncoder(w).Encode(resp)
}

func handler11(w http.ResponseWriter, r *http.Request) {
	var req Payload
	_ = json.NewDecoder(r.Body).Decode(&req)
	resp := f11(req)
	_ = json.NewEncoder(w).Encode(resp)
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
	_ = http.ListenAndServe(":8080", mux)
}
