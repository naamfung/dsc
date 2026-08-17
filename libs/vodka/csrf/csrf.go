package csrf

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"dsc/libs/vodka"
	"dsc/libs/vodka/libraries/gommon/random"
	"dsc/libs/vodka/skipper"
)

type (
	// CSRFConfig defines the config for CSRF middleware.
	CSRFConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper

		// TokenLength is the length of the generated token.
		TokenLength uint8 `json:"token_length"`
		// Optional. Default value 32.

		// TokenLookup is a string in the form of "<source>:<key>" that is used
		// to extract token from the request.
		// Optional. Default value "header:X-CSRF-Token".
		// Possible values:
		// - "header:<name>"
		// - "form:<name>"
		// - "query:<name>"
		// - "cookie:<name>"
		TokenLookup string `json:"token_lookup"`

		// Context key to store generated CSRF token into context.
		// Optional. Default value "csrf".
		ContextKey string `json:"context_key"`

		// Name of the CSRF cookie. This cookie will store CSRF token.
		// Optional. Default value "_csrf".
		CookieName string `json:"cookie_name"`

		// Domain of the CSRF cookie.
		// Optional. Default value none.
		CookieDomain string `json:"cookie_domain"`

		// Path of the CSRF cookie.
		// Optional. Default value none.
		CookiePath string `json:"cookie_path"`

		// Max age (in seconds) of the CSRF cookie.
		// Optional. Default value 86400 (24hr).
		CookieMaxAge int `json:"cookie_max_age"`

		// Indicates if CSRF cookie is secure.
		// Optional. Default value false.
		CookieSecure bool `json:"cookie_secure"`

		// Indicates if CSRF cookie is HTTP only.
		// Optional. Default value false.
		CookieHTTPOnly bool `json:"cookie_http_only"`
	}
)

var (
	// DefaultCSRFConfig is the default CSRF middleware config.
	DefaultCSRFConfig = CSRFConfig{
		Skipper:      skipper.DefaultSkipper,
		TokenLength:  32,
		TokenLookup:  "header:" + vodka.HeaderXCSRFToken,
		ContextKey:   "csrf",
		CookieName:   "_csrf",
		CookieMaxAge: 86400,
	}
)

var (
	errMissingHeader = errors.New("[CSRF] Missing csrf token in header")
	errMissingQuery  = errors.New("[CSRF] Missing csrf token in query")
	errMissingParam  = errors.New("[CSRF] Missing csrf token in param")
	errMissingForm   = errors.New("[CSRF] Missing csrf token in form")
	errInvalidToken  = errors.New("[CSRF] The csrf token is invalid")
)

// CSRF returns a Cross-Site Request Forgery (CSRF) middleware.
// See: https://en.wikipedia.org/wiki/Cross-site_request_forgery
func CSRF() vodka.Handler {
	c := DefaultCSRFConfig
	return CSRFWithConfig(c)
}

// CSRFWithConfig returns a CSRF middleware with config.
// See `CSRF()`.
func CSRFWithConfig(config CSRFConfig) vodka.Handler {
	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultCSRFConfig.Skipper
	}
	if config.TokenLength == 0 {
		config.TokenLength = DefaultCSRFConfig.TokenLength
	}
	if config.TokenLookup == "" {
		config.TokenLookup = DefaultCSRFConfig.TokenLookup
	}
	if config.ContextKey == "" {
		config.ContextKey = DefaultCSRFConfig.ContextKey
	}
	if config.CookieName == "" {
		config.CookieName = DefaultCSRFConfig.CookieName
	}
	if config.CookieMaxAge == 0 {
		config.CookieMaxAge = DefaultCSRFConfig.CookieMaxAge
	}

	// Initialize
	var parts = strings.Split(config.TokenLookup, ":")
	extractor := fromHeader(parts[1]) // By default, we extract from a header
	switch parts[0] {
	case "form":
		extractor = fromForm(parts[1])
	case "query":
		extractor = fromQuery(parts[1])
	case "param":
		extractor = fromParam(parts[1])
	default:
		if parts[0] == "cookie" {
			extractor = fromCookie(parts[1])
		}
	}

	return func(c *vodka.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		var token string
		k, err := c.Request.Cookie(config.CookieName)
		if err != nil {
			// Generate token
			token = random.String(config.TokenLength)
		} else {
			// Reuse token
			token = k.Value
		}

		switch c.Request.Method {
		case vodka.GET, vodka.HEAD, vodka.OPTIONS, vodka.TRACE: // Ignore csrf token for these methods
		default:
			// Validate token only for requests which are not defined as 'safe' by RFC7231
			clientToken, err := extractor(c)
			if err != nil {
				return err
			}
			if !validateCSRFToken(token, clientToken) {
				return vodka.NewHTTPError(vodka.StatusForbidden, errInvalidToken)
			}
		}

		// Set CSRF cookie
		cookie := c.NewCookie()
		cookie.Name = config.CookieName
		cookie.Value = token
		if config.CookiePath != "" {
			cookie.Path = config.CookiePath
		}
		if config.CookieDomain != "" {
			cookie.Domain = config.CookieDomain
		}
		cookie.Expires = time.Now().Add(time.Duration(config.CookieMaxAge) * time.Second)
		cookie.Secure = config.CookieSecure
		cookie.HttpOnly = config.CookieHTTPOnly
		c.SetCookie(cookie)

		// Store token in the context
		c.Set(config.ContextKey, token)

		// Protect clients from caching the response
		c.Response.Header().Add(vodka.HeaderVary, vodka.HeaderCookie)
		return c.Next()
	}
}

// fromHeader returns a function that extracts token from the request header.
func fromHeader(header string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.RequestHeader(header)
		if token == "" {
			return "", errMissingHeader
		}
		return token, nil
	}
}

// fromParam returns a function that extracts token from the url param string.
func fromParam(param string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.Param(param).String()
		if token == "" {
			return "", errMissingParam
		}
		return token, nil
	}
}

// fromCookie returns a function that extracts token from the named cookie.
func fromCookie(name string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		cookie, err := c.GetCookie(name)
		if err != nil || cookie == nil {
			return "", fmt.Errorf("empty csrf token in cookie:%v", err)
		}
		return cookie.Value, nil
	}
}

// fromForm returns a function that extracts a token from a multipart-form.
func fromForm(param string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.FormValue(param)
		if token == "" {
			return "", errMissingForm
		}
		return token, nil
	}
}

// fromQuery returns a function that extracts token from the query string.
func fromQuery(param string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.Query(param)
		if token == "" {
			return "", errMissingQuery
		}
		return token, nil
	}
}

func validateCSRFToken(token, clientToken string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(clientToken)) == 1
}
