package requestid

import (
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestRequestID(t *testing.T) {
	e := vodka.New()
	req := httptest.NewRequest(vodka.GET, "/", nil)
	rec := httptest.NewRecorder()
	handler := func(c *vodka.Context) error {
		return c.String("test", vodka.StatusOK)
	}
	c := e.NewContext(req, rec, handler)
	h := RequestIDWithConfig(RequestIDConfig{})
	h(c)
	assert.Len(t, rec.Header().Get(vodka.HeaderXRequestID), 32)

	// Custom generator
	c = e.NewContext(req, rec, handler)
	h = RequestIDWithConfig(RequestIDConfig{
		Generator: func() string { return "customGenerator" },
	})
	h(c)
	assert.Equal(t, rec.Header().Get(vodka.HeaderXRequestID), "customGenerator")
}
