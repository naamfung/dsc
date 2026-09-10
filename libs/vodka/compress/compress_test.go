package compress

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestGzip(t *testing.T) {
	req := httptest.NewRequest(vodka.GET, "/", nil)
	rec := httptest.NewRecorder()
	m := vodka.New()
	m.Use(Gzip())
	m.Get("/", func(c *vodka.Context) error {
		return c.String("test") // For Content-Type sniffing
	})
	// Skip if no Accept-Encoding header
	m.ServeHTTP(rec, req)
	assert.Equal(t, "test", rec.Body.String())

	// Gzip
	req = httptest.NewRequest(vodka.GET, "/", nil)
	req.Header.Set(vodka.HeaderAcceptEncoding, gzipScheme)
	rec = httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	assert.Equal(t, gzipScheme, rec.Header().Get(vodka.HeaderContentEncoding))
	assert.Contains(t, rec.Header().Get(vodka.HeaderContentType), vodka.MIMETextPlain)
	r, err := gzip.NewReader(rec.Body)
	defer r.Close()
	if assert.NoError(t, err) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(r)
		assert.Equal(t, "test", buf.String())
	}
}

func TestGzipNoContent(t *testing.T) {
	e := vodka.New()
	req := httptest.NewRequest(vodka.GET, "/", nil)
	req.Header.Set(vodka.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec, func(c *vodka.Context) error {
		return c.NoContent(vodka.StatusNoContent)
	})
	h := Gzip()
	if assert.NoError(t, h(c)) {
		assert.Empty(t, rec.Header().Get(vodka.HeaderContentEncoding))
		assert.Empty(t, rec.Header().Get(vodka.HeaderContentType))
		assert.Equal(t, 0, len(rec.Body.Bytes()))
	}
}

func TestGzipErrorReturned(t *testing.T) {
	e := vodka.New()
	e.Use(Gzip())
	e.Get("/", func(c *vodka.Context) error {
		return vodka.ErrNotFound
	})
	req := httptest.NewRequest(vodka.GET, "/", nil)
	req.Header.Set(vodka.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, rec.Header().Get(vodka.HeaderContentEncoding))
}

// Issue #806
func TestGzipWithStatic(t *testing.T) {
	e := vodka.New()
	e.Use(Gzip())
	e.Static("/test", "../public/images")
	req := httptest.NewRequest(vodka.GET, "/test/vodka.jpg", nil)
	req.Header.Set(vodka.HeaderAcceptEncoding, gzipScheme)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	// Data is written out in chunks when Content-Length == "", so only
	// validate the content length if it's not set.
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		assert.Equal(t, cl, rec.Body.Len())
	}
	r, err := gzip.NewReader(rec.Body)
	assert.NoError(t, err)
	defer r.Close()
	want, err := ioutil.ReadFile("../public/images/vodka.jpg")
	if assert.NoError(t, err) {
		var buf bytes.Buffer
		buf.ReadFrom(r)
		assert.Equal(t, want, buf.Bytes())
	}
}
