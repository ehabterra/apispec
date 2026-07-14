package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/mux"
)

// Product is served by the gin API.
type Product struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}

// AdminReport is served by the gorilla/mux admin router.
type AdminReport struct {
	Users  int `json:"users"`
	Orders int `json:"orders"`
}

func listProducts(c *gin.Context) {
	c.JSON(http.StatusOK, []Product{})
}

func createProduct(c *gin.Context) {
	var p Product
	_ = c.ShouldBindJSON(&p)
	c.JSON(http.StatusCreated, p)
}

func adminReport(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(AdminReport{})
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	_ = mux.Vars(r)["id"]
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	// Public API on gin …
	g := gin.Default()
	g.GET("/products", listProducts)
	g.POST("/products", createProduct)

	// … and an internal admin surface on gorilla/mux, in the same binary.
	m := mux.NewRouter()
	m.HandleFunc("/admin/report", adminReport).Methods("GET")
	m.HandleFunc("/admin/users/{id}", deleteUser).Methods("DELETE")

	go func() { _ = http.ListenAndServe(":9091", m) }()
	_ = g.Run(":8080")
}
