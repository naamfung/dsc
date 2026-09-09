package cors

import (
	"net/http"
	"strconv"
	"strings"

	"dsc/libs/vodka"
	"dsc/libs/vodka/skipper"
)

type (
	// CORSConfig defines the config for CORS middleware.
	CORSConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper

		// AllowOrigin defines a list of origins that may access the resource.
		// Optional. Default value []string{"*"}.
		AllowOrigins []string `json:"allow_origins"`

		// AllowMethods defines a list methods allowed when accessing the resource.
		// This is used in response to a preflight request.
		// Optional. Default value DefaultCORSConfig.AllowMethods.
		AllowMethods []string `json:"allow_methods"`

		// AllowHeaders defines a list of request headers that can be used when
		// making the actual request. This in response to a preflight request.
		// Optional. Default value []string{}.
		AllowHeaders []string `json:"allow_headers"`

		// AllowCredentials indicates whether or not the response to the request
		// can be exposed when the credentials flag is true. When used as part of
		// a response to a preflight request, this indicates whether or not the
		// actual request can be made using credentials.
		// Optional. Default value false.
		AllowCredentials bool `json:"allow_credentials"`

		// ExposeHeaders defines a whitelist headers that clients are allowed to
		// access.
		// Optional. Default value []string{}.
		ExposeHeaders []string `json:"expose_headers"`

		// MaxAge indicates how long (in seconds) the results of a preflight request
		// can be cached.
		// Optional. Default value 0.
		MaxAge int `json:"max_age"`
	}
)

var (
	// DefaultCORSConfig is the default CORS middleware config.
	DefaultCORSConfig = CORSConfig{
		Skipper:      skipper.DefaultSkipper,
		AllowOrigins: []string{"*"},
		AllowMethods: []string{vodka.GET, vodka.HEAD, vodka.PUT, vodka.PATCH, vodka.POST, vodka.DELETE},
	}
)

// CORS returns a Cross-Origin Resource Sharing (CORS) middleware.
// See: https://developer.mozilla.org/en/docs/Web/HTTP/Access_control_CORS
func CORS() vodka.Handler {
	return CORSWithConfig(DefaultCORSConfig)
}

// CORSWithConfig returns a CORS middleware with config.
// See: `CORS()`.
func CORSWithConfig(config CORSConfig) vodka.Handler {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultCORSConfig.Skipper
	}
	if len(config.AllowOrigins) == 0 {
		config.AllowOrigins = DefaultCORSConfig.AllowOrigins
	}
	if len(config.AllowMethods) == 0 {
		config.AllowMethods = DefaultCORSConfig.AllowMethods
	}

	allowMethods := strings.Join(config.AllowMethods, ",")
	allowHeaders := strings.Join(config.AllowHeaders, ",")
	exposeHeaders := strings.Join(config.ExposeHeaders, ",")
	maxAge := strconv.Itoa(config.MaxAge)

	return func(c *vodka.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		req := c.Request
		res := c.Response
		origin := req.Header.Get(vodka.HeaderOrigin)
		allowOrigin := ""

		// Check allowed origins
		for _, o := range config.AllowOrigins {
			if o == "*" || o == origin {
				allowOrigin = o
				break
			}
		}

		// Simple request
		if req.Method != vodka.OPTIONS {
			res.Header().Add(vodka.HeaderVary, vodka.HeaderOrigin)
			res.Header().Set(vodka.HeaderAccessControlAllowOrigin, allowOrigin)
			if config.AllowCredentials {
				res.Header().Set(vodka.HeaderAccessControlAllowCredentials, "true")
			}
			if exposeHeaders != "" {
				res.Header().Set(vodka.HeaderAccessControlExposeHeaders, exposeHeaders)
			}
			return c.Next()
		}

		// Preflight request
		res.Header().Add(vodka.HeaderVary, vodka.HeaderOrigin)
		res.Header().Add(vodka.HeaderVary, vodka.HeaderAccessControlRequestMethod)
		res.Header().Add(vodka.HeaderVary, vodka.HeaderAccessControlRequestHeaders)
		res.Header().Set(vodka.HeaderAccessControlAllowOrigin, allowOrigin)
		res.Header().Set(vodka.HeaderAccessControlAllowMethods, allowMethods)
		if config.AllowCredentials {
			res.Header().Set(vodka.HeaderAccessControlAllowCredentials, "true")
		}
		if allowHeaders != "" {
			res.Header().Set(vodka.HeaderAccessControlAllowHeaders, allowHeaders)
		} else {
			h := req.Header.Get(vodka.HeaderAccessControlRequestHeaders)
			if h != "" {
				res.Header().Set(vodka.HeaderAccessControlAllowHeaders, h)
			}
		}
		if config.MaxAge > 0 {
			res.Header().Set(vodka.HeaderAccessControlMaxAge, maxAge)
		}
		return c.NoContent(http.StatusNoContent)
	}
}
