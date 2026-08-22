package core

import "testing"

func TestPackage(t *testing.T) {
	e := NewEngine()
	if e == nil {
		t.Error("NewEngine should not return nil")
	}
}
