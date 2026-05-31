package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/ehabterra/apispec/testdata/echo_handler_factory/api"
	"github.com/ehabterra/apispec/testdata/echo_handler_factory/models"
	"github.com/ehabterra/apispec/testdata/echo_handler_factory/util"
)

type userHandlers struct{}

// New returns the concrete implementation behind the api.Handlers interface.
func New() api.Handlers { return &userHandlers{} }

// Create is a handler factory: request binding goes through the util.ReadRequest
// wrapper (so the request type must be traced through a parameter), and the
// response is written directly with c.JSON.
func (h *userHandlers) Create() echo.HandlerFunc {
	return func(c echo.Context) error {
		u := &models.User{}
		if err := util.ReadRequest(c, u); err != nil {
			return err
		}
		return c.JSON(http.StatusCreated, u)
	}
}

func (h *userHandlers) Get() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, &models.User{})
	}
}

// Login uses a *function-local* request type, declared inside the method before
// the returned closure. Such a type is not a package-level declaration, so it
// must be captured from the function body or the request body resolves to a
// dangling $ref (an unresolved placeholder).
func (h *userHandlers) Login() echo.HandlerFunc {
	type Login struct {
		Email    string `json:"email" validate:"omitempty,email"`
		Password string `json:"password" validate:"required,gte=6"`
	}
	return func(c echo.Context) error {
		login := &Login{}
		if err := util.ReadRequest(c, login); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, &models.User{})
	}
}
