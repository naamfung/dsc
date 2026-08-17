package kauth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestKeyAuth(t *testing.T) {
	e := vodka.New()
	req := httptest.NewRequest(vodka.GET, "/", nil)
	res := httptest.NewRecorder()
	c := e.NewContext(req, res, func(c *vodka.Context) error {
		return c.String("test", vodka.StatusOK)
	})
	config := KeyAuthConfig{
		Validator: func(key string, c *vodka.Context) (error, bool) {
			return nil, key == "valid-key"
		},
	}
	h := KeyAuthWithConfig(config)

	// Valid key
	auth := DefaultKeyAuthConfig.AuthScheme + " " + "valid-key"
	req.Header.Set(vodka.HeaderAuthorization, auth)
	assert.NoError(t, h(c))

	// Invalid key
	auth = DefaultKeyAuthConfig.AuthScheme + " " + "invalid-key"
	req.Header.Set(vodka.HeaderAuthorization, auth)
	err := h(c)
	assert.Error(t, err)
	he, ok := err.(*vodka.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, he.StatusCode())

	// Missing Authorization header
	req.Header.Del(vodka.HeaderAuthorization)
	he = h(c).(*vodka.HTTPError)
	assert.Equal(t, http.StatusBadRequest, he.StatusCode())

	// Key from custom header
	config.KeyLookup = "header:API-Key"
	c = e.NewContext(req, res, func(c *vodka.Context) error {
		return c.String("test", vodka.StatusOK)
	})
	h = KeyAuthWithConfig(config)
	req.Header.Set("API-Key", "valid-key")
	assert.NoError(t, h(c))

	// Key from query string
	config.KeyLookup = "query:key"
	c = e.NewContext(req, res, func(c *vodka.Context) error {
		return c.String("test", vodka.StatusOK)
	})
	h = KeyAuthWithConfig(config)
	q := req.URL.Query()
	q.Add("key", "valid-key")
	req.URL.RawQuery = q.Encode()
	assert.NoError(t, h(c))
}
