package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Item struct {
	ID string `json:"id"`
}

// One assignment: not ambiguous.
func short(c echo.Context) error {
	code := http.StatusConflict
	return c.JSON(code, Item{})
}

func str(c echo.Context) error {
	code := http.StatusConflict
	return c.String(code, "dup")
}

func varDecl(c echo.Context) error {
	var code = http.StatusAccepted
	return c.JSON(code, Item{})
}

func literal(c echo.Context) error {
	code := 418
	return c.JSON(code, Item{})
}

// Reassigned on one path: the later write is what is sent.
func reassigned(c echo.Context) error {
	code := http.StatusOK
	code = http.StatusCreated
	return c.JSON(code, Item{})
}

// Branch-assigned: one response per value (already supported, #39).
func branched(c echo.Context) error {
	code := http.StatusOK
	if c.QueryParam("new") != "" {
		code = http.StatusCreated
	}
	return c.JSON(code, Item{})
}

// 0 is not a status: the zero-value arm is not 201, and is no status at all.
func zeroDefault(c echo.Context) error {
	code := 0
	if c.QueryParam("new") != "" {
		code = http.StatusCreated
	}
	return c.JSON(code, Item{})
}

// A sentinel alone.
func zeroOnly(c echo.Context) error {
	code := 0
	return c.JSON(code, Item{})
}

// Not knowable: stays default (golden rule #7).
func computed(c echo.Context) error {
	code := statusFor(c)
	return c.JSON(code, Item{})
}

// A call that merely MENTIONS a status: the default applies only when no
// status is passed, so 202 would be a guess.
func optionalArg(c echo.Context) error {
	code := orDefault(statusesFrom(c), http.StatusAccepted)
	return c.JSON(code, Item{})
}

func orDefault(given []int, def int) int {
	if len(given) > 0 {
		return given[0]
	}
	return def
}

func statusesFrom(c echo.Context) []int {
	if c.QueryParam("gone") != "" {
		return []int{http.StatusGone}
	}
	return nil
}

func statusFor(c echo.Context) int {
	if c.QueryParam("x") != "" {
		return http.StatusOK
	}
	return http.StatusAccepted
}

type Problem struct {
	Detail string `json:"detail"`
}

// The early-return idiom: the 400 arm returns, so its value never reaches the
// success call, and the success value never reaches the 400 call.
func earlyReturn(c echo.Context) error {
	code := http.StatusOK
	if c.QueryParam("bad") != "" {
		code = http.StatusBadRequest
		return c.JSON(code, Problem{})
	}
	return c.JSON(code, Item{})
}

func main() {
	e := echo.New()
	// A closure handler reads its own local.
	e.GET("/closure", func(c echo.Context) error {
		code := http.StatusConflict
		return c.JSON(code, Item{})
	})
	e.GET("/early-return", earlyReturn)
	e.GET("/short", short)
	e.GET("/string", str)
	e.GET("/var-decl", varDecl)
	e.GET("/literal", literal)
	e.GET("/reassigned", reassigned)
	e.GET("/branched", branched)
	e.GET("/computed", computed)
	e.GET("/zero-default", zeroDefault)
	e.GET("/zero-only", zeroOnly)
	e.GET("/optional-arg", optionalArg)
	_ = e.Start(":8080")
}
