// Package vodka is a high productive and modular web framework in Golang.

package fault

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"dsc/libs/vodka"

	"github.com/stretchr/testify/assert"
)

func TestErrorHandler(t *testing.T) {
	var buf bytes.Buffer
	h := ErrorHandler(getLogger(&buf))

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/", nil)
	m := vodka.New()
	c := m.NewContext(req, res, h, handler1, handler2)
	assert.Nil(t, c.Next())
	assert.Equal(t, vodka.StatusInternalServerError, res.Code)
	assert.Equal(t, "abc", res.Body.String())
	assert.Equal(t, "abc", buf.String())

	buf.Reset()
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = m.NewContext(req, res, h, handler2)
	assert.Nil(t, c.Next())
	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "test", res.Body.String())
	assert.Equal(t, "", buf.String())

	buf.Reset()
	h = ErrorHandler(getLogger(&buf), convertError)
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = m.NewContext(req, res, h, handler1, handler2)
	assert.Nil(t, c.Next())
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Equal(t, "123", res.Body.String())
	assert.Equal(t, "abc", buf.String())

	buf.Reset()
	h = ErrorHandler(nil)
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = m.NewContext(req, res, h, handler1, handler2)
	assert.Nil(t, c.Next())
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Equal(t, "abc", res.Body.String())
	assert.Equal(t, "", buf.String())
}

func Test_writeError(t *testing.T) {
	m := vodka.New()

	// 普通错误 → 500 + 错误文本（对齐当前 HandleError 行为：不追加换行）
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/", nil)
	c := m.NewContext(req, res)
	c.HandleError(errors.New("abc"))
	assert.Equal(t, http.StatusInternalServerError, res.Code)
	assert.Equal(t, "abc", res.Body.String())

	// HTTPError → 其状态码 + 消息
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = m.NewContext(req, res)
	c.HandleError(vodka.NewHTTPError(http.StatusNotFound, "xyz"))
	assert.Equal(t, http.StatusNotFound, res.Code)
	assert.Equal(t, "xyz", res.Body.String())
}

func convertError(c *vodka.Context, err error) error {
	return errors.New("123")
}
