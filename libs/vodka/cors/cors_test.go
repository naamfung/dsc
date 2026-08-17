package cors

import (
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestCORS(t *testing.T) {
	e := vodka.New()

	// Wildcard origin
	req := httptest.NewRequest(vodka.GET, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec, vodka.NotFoundHandler)
	h := CORS()
	h(c)
	assert.Equal(t, "*", rec.Header().Get(vodka.HeaderAccessControlAllowOrigin))

	// Allow origins
	req = httptest.NewRequest(vodka.GET, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec, vodka.NotFoundHandler)
	h = CORSWithConfig(CORSConfig{
		AllowOrigins: []string{"localhost"},
	})
	req.Header.Set(vodka.HeaderOrigin, "localhost")
	h(c)
	assert.Equal(t, "localhost", rec.Header().Get(vodka.HeaderAccessControlAllowOrigin))

	// Preflight request
	req = httptest.NewRequest(vodka.OPTIONS, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec, vodka.NotFoundHandler)
	req.Header.Set(vodka.HeaderOrigin, "localhost")
	req.Header.Set(vodka.HeaderContentType, vodka.MIMEApplicationJSON)
	cors := CORSWithConfig(CORSConfig{
		AllowOrigins:     []string{"localhost"},
		AllowCredentials: true,
		MaxAge:           3600,
	})
	cors(c)
	assert.Equal(t, "localhost", rec.Header().Get(vodka.HeaderAccessControlAllowOrigin))
	assert.NotEmpty(t, rec.Header().Get(vodka.HeaderAccessControlAllowMethods))
	assert.Equal(t, "true", rec.Header().Get(vodka.HeaderAccessControlAllowCredentials))
	assert.Equal(t, "3600", rec.Header().Get(vodka.HeaderAccessControlMaxAge))
}
