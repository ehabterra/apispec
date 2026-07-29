// Fixture: many routes registered inside ONE chi group closure, every handler a
// method on a shared handler struct, every response written through ONE shared
// helper method.
//
// This is the shape of issue #224. The instance cap bounds copies of a callee
// within an "instance scope", and the scope is the nearest ARGUMENT-node
// ancestor. The single `json.NewEncoder(w).Encode(v)` inside `respond` is ONE
// call site reached by every route, so it is counted once per route against
// whatever scope the routes share. When that scope is the group closure rather
// than the handler, the budget is a per-GROUP budget and every route past the
// cap silently loses its response body.
//
// Each handler returns a DIFFERENT type, so the payload has to be traced per
// route: a starved route shows up as a missing schema, not as a wrong one.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type service struct{}

type handler struct {
	svc *service
}

// respond is the one house helper. The Encode call inside it is a single call
// site shared by every route in the project.
func (h *handler) respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type Alpha struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Bravo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Charlie struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Delta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Echo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Foxtrot struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Golf struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Hotel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type India struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Juliet struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Kilo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Lima struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Mike struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type November struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Oscar struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *service) Alpha() (Alpha, error) { return Alpha{}, nil }

func (s *service) Bravo() (Bravo, error) { return Bravo{}, nil }

func (s *service) Charlie() (Charlie, error) { return Charlie{}, nil }

func (s *service) Delta() (Delta, error) { return Delta{}, nil }

func (s *service) Echo() (Echo, error) { return Echo{}, nil }

func (s *service) Foxtrot() (Foxtrot, error) { return Foxtrot{}, nil }

func (s *service) Golf() (Golf, error) { return Golf{}, nil }

func (s *service) Hotel() (Hotel, error) { return Hotel{}, nil }

func (s *service) India() (India, error) { return India{}, nil }

func (s *service) Juliet() (Juliet, error) { return Juliet{}, nil }

func (s *service) Kilo() (Kilo, error) { return Kilo{}, nil }

func (s *service) Lima() (Lima, error) { return Lima{}, nil }

func (s *service) Mike() (Mike, error) { return Mike{}, nil }

func (s *service) November() (November, error) { return November{}, nil }

func (s *service) Oscar() (Oscar, error) { return Oscar{}, nil }

func (h *handler) getAlpha(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Alpha()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getBravo(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Bravo()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getCharlie(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Charlie()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getDelta(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Delta()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getEcho(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Echo()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getFoxtrot(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Foxtrot()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getGolf(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Golf()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getHotel(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Hotel()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getIndia(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.India()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getJuliet(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Juliet()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getKilo(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Kilo()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getLima(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Lima()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getMike(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Mike()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getNovember(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.November()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func (h *handler) getOscar(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Oscar()
	if err != nil {
		h.respond(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.respond(w, http.StatusOK, res)
}

func main() {
	h := &handler{svc: &service{}}
	r := chi.NewRouter()
	r.Route("/api", func(api chi.Router) {
		api.Get("/alpha", h.getAlpha)
		api.Get("/bravo", h.getBravo)
		api.Get("/charlie", h.getCharlie)
		api.Get("/delta", h.getDelta)
		api.Get("/echo", h.getEcho)
		api.Get("/foxtrot", h.getFoxtrot)
		api.Get("/golf", h.getGolf)
		api.Get("/hotel", h.getHotel)
		api.Get("/india", h.getIndia)
		api.Get("/juliet", h.getJuliet)
		api.Get("/kilo", h.getKilo)
		api.Get("/lima", h.getLima)
		api.Get("/mike", h.getMike)
		api.Get("/november", h.getNovember)
		api.Get("/oscar", h.getOscar)
	})
	_ = http.ListenAndServe(":8080", r)
}
