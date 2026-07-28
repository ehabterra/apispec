// Fixture: a parameter's name is the string a CLIENT sends, so it has to be
// statically known — and the ways it becomes known (or stays unknown) all appear
// in one real handler.
//
// Before this was enforced, an unknown name was filled in with whatever the Go
// expression rendered to, producing parameters no request could ever match:
// `name: github.com/labstack/echo.HeaderXRequestID` for a constant from a
// package outside the analysed set, and `name: string` — the parameter's TYPE —
// for a name passed down through a wrapper.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HeaderTenant is declared HERE, so its value is recorded and resolvable.
const HeaderTenant = "X-Tenant"

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// readFile takes the form field name as a PARAMETER. The literal lives at the
// call site, so the name is knowable only by tracing the parameter back — the
// shape that used to document the file part as "string".
func readFile(c echo.Context, field string) (string, error) {
	f, err := c.FormFile(field)
	if err != nil {
		return "", err
	}
	return f.Filename, nil
}

func upload(c echo.Context) error {
	// Knowable through the wrapper parameter -> the part is named "avatar".
	name, err := readFile(c, "avatar")
	if err != nil {
		return c.JSON(http.StatusBadRequest, Item{})
	}

	// Knowable: a constant this project declares -> the header is "X-Tenant",
	// not "HeaderTenant".
	_ = c.Request().Header.Get(HeaderTenant)

	// NOT knowable: the constant belongs to echo, which is never analysed, so
	// its value is unavailable. No parameter can be documented for it.
	_ = c.Request().Header.Get(echo.HeaderXRequestID)

	// Knowable: a plain literal.
	_ = c.QueryParam("scope")

	return c.JSON(http.StatusOK, Item{Name: name})
}

func main() {
	e := echo.New()
	e.POST("/items/upload", upload)
	_ = e.Start(":8080")
}
