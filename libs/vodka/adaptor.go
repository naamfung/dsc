package vodka

import (
	"net/http"
)

// WrapHTTPHandler wraps `http.Handler` into `vodka.Handler`.
func WrapHTTPHandler(handler http.Handler) Handler {
	return func(c *Context) error {
		handler.ServeHTTP(c.Response, c.Request)
		return nil
	}
}
