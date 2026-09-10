// Package vodka is a high productive and modular web framework in Golang.

package vodka

import (
	"fmt"
	"net/http"
)

// DataWriter is used by Context.Write() to write arbitrary data into an HTTP response.
type DataWriter interface {
	// SetHeader sets necessary response headers.
	SetHeader(http.ResponseWriter)
	// Write writes the given data into the response.
	Write(http.ResponseWriter, interface{}) error
}

// DefaultDataWriter writes the given data in an HTTP response.
// If the data is neither string nor byte array, it will use fmt.Fprint() to write it into the response.
var DefaultDataWriter DataWriter = &dataWriter{}

type dataWriter struct{}

func (w *dataWriter) SetHeader(res http.ResponseWriter) {}

func (w *dataWriter) Write(res http.ResponseWriter, data interface{}) error {
	var bytes []byte
	switch data := data.(type) { // 将类型断言的结果赋值给 data
	case []byte:
		bytes = data
	case string:
		bytes = []byte(data)
	default:
		if data != nil {
			_, err := fmt.Fprint(res, data)
			return err
		}
	}
	_, err := res.Write(bytes)
	return err
}
