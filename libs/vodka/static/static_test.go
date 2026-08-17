package static

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestStatic(t *testing.T) {
	e := vodka.New()
	req := httptest.NewRequest(vodka.GET, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec, vodka.NotFoundHandler)
	config := StaticConfig{
		Root: "../public",
	}

	// Directory
	h := StaticWithConfig(config)
	if assert.NoError(t, h(c)) {
		assert.Contains(t, rec.Body.String(), "Vodka")
	}

	// File found
	req = httptest.NewRequest(vodka.GET, "/images/vodka.jpg", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	//m := vodka.New()
	//m.Use(StaticWithConfig(config))
	//m.ServeHTTP(rec, req)
	if assert.NoError(t, h(c)) {
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.NotZero(t, rec.Body.Len())
		assert.Equal(t, fmt.Sprintf("%v", rec.Body.Len()), rec.Header().Get(vodka.HeaderContentLength))
	}

	// File not found
	req = httptest.NewRequest(vodka.GET, "/none", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec, vodka.NotFoundHandler)
	err := h(c)
	assert.Error(t, err)
	he, ok := err.(*vodka.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusNotFound, he.StatusCode())

	// HTML5
	req = httptest.NewRequest(vodka.GET, "/random", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec, vodka.NotFoundHandler)
	config.HTML5 = true
	err = StaticWithConfig(config)(c)
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "Vodka")
	}

	// Browse
	req = httptest.NewRequest(vodka.GET, "/", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec, vodka.NotFoundHandler)
	config.Root = "../public/certs"
	config.Browse = true
	static := StaticWithConfig(config)
	err = static(c)
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "cert.pem")
	}
}
