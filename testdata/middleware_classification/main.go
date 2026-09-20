// Every middleware in a Use/group/per-route slot used to be announced as "auth
// middleware not mapped to a security scheme" — and on a normal service most of
// it is logging, recovery, CORS, rate limiting and timeouts. A warning that is
// mostly false trains people past the true one, and the true one matters: its
// routes are documented as PUBLIC (issue #520).
//
// What separates them is what the middleware's BODY does, not what it is
// called — a denylist of "logger"/"cors" would be the guess golden rule #9
// forbids. So the ones here are deliberately named for their shape rather than
// their job, and two of the non-auth ones refuse requests, which is the signal
// that is NOT sufficient on its own.
package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// MAPPED auth middleware: resolved, never reported.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UNMAPPED house auth: reads a credential -> MUST be reported.
func houseAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UNMAPPED auth through a helper one hop down -> MUST be reported.
func tokenOf(r *http.Request) string { return r.Header.Get("Authorization") }

func delegatingAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tokenOf(r) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UNMAPPED basic auth -> MUST be reported (no name given at all).
func basicGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Not auth: logs only.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println(r.Method, r.URL.Path, r.Header.Get("X-Request-Id"))
		next.ServeHTTP(w, r)
	})
}

// Not auth: refuses, but reads no credential.
func rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Burst") != "" {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func list(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }

func main() {
	r := chi.NewRouter()
	r.Use(requestLogger, rateLimit, authMiddleware, houseAuth, delegatingAuth, basicGuard)
	r.Get("/items", list)
	_ = http.ListenAndServe(":8080", r)
}
