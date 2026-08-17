package skipper

import "dsc/libs/vodka"

type (
	// Skipper defines a function to skip middleware. Returning true skips processing
	// the middleware.
	Skipper func(c *vodka.Context) bool
)

// defaultSkipper returns false which processes the middleware.
func DefaultSkipper(c *vodka.Context) bool {
	return false
}
