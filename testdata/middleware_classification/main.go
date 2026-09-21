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
	"errors"
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

// Session auth: reads a cookie and REDIRECTS to a login page. Writes no 401
// ever, so the status signal alone would miss it.
func sessionAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("session"); err != nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Auth on a HOUSE credential name no table can predict, refusing with 401. The
// credential signal alone would miss it; the status is what catches it.
func signatureAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Acme-Request-Signature") == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Not authentication: an authorisation gate stacked after the real auth
// middleware. It reads a role the auth middleware already put in the context
// and refuses with 403 — "not allowed", which no security scheme describes.
type roleKey struct{}

func roleGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if role, _ := r.Context().Value(roleKey{}).(string); role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Auth that hands the status decision to a shared renderer: nothing in its own
// call graph writes 401. The credential read is what catches it.
var errNoToken = errors.New("no token")

func renderErr(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func cookielessAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Auth-Token") == "" {
			renderErr(w, errNoToken)
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

// The three shapes that carry a credential-looking value WITHOUT reading one.
// A signal has to come from what the call DOES; a literal on its own is not
// evidence, or all three of these read as authentication.

func observe(code int) {}

// Logs the literal word, never reads the header.
func logsTheWord(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("Authorization", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// SETS an outbound Authorization header — the opposite of reading one.
func addsUpstreamAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer upstream")
		next.ServeHTTP(w, r)
	})
}

// Passes 403 to a metric, never to a writer.
func countsForbidden(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observe(http.StatusForbidden)
		next.ServeHTTP(w, r)
	})
}

func list(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }

func main() {
	r := chi.NewRouter()
	r.Use(requestLogger, rateLimit, authMiddleware, houseAuth, delegatingAuth, basicGuard,
		sessionAuth, signatureAuth, cookielessAuth,
		logsTheWord, addsUpstreamAuth, countsForbidden, roleGate)
	r.Get("/items", list)
	_ = http.ListenAndServe(":8080", r)
}
