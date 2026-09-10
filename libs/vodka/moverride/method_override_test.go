package moverride_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/moverride"
	"testing"
)

func TestMethodOverride(t *testing.T) {
	m := vodka.New()
	m.Use(moverride.MethodOverrideWithConfig(moverride.MethodOverrideConfig{
		Getter: moverride.MethodFromForm("_method"),
	}))
	go m.Listen(":9000")
}
