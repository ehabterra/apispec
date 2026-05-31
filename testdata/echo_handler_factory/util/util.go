package util

import "github.com/labstack/echo/v4"

// ReadRequest is a custom request-binding wrapper around echo.Context.Bind —
// the indirection real projects use (e.g. AleksK1NG's utils.ReadRequest). The
// concrete request type is only known at ReadRequest's *call site*; inside,
// the bound value is the untyped parameter `v`.
func ReadRequest(c echo.Context, v interface{}) error {
	return c.Bind(v)
}
