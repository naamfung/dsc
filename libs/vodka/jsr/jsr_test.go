package jsr

import (
	"testing"
)

func TestJSR(t *testing.T) {
	v, e := JSR().RunString("1 + 2")
	if e != nil {
		panic(e)
	}
	if i, okay := v.Export().(int64); okay {
		if i != 3 {
			panic("i!=3")
		}
	} else {
		panic("jsr error")
	}
}
