package blimit_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/blimit"
	"dsc/libs/vodka/skipper"
	"testing"
)

func TestBodyLimit(t *testing.T) {
	m := vodka.New()
	m.Use(blimit.BodyLimit("2M"))
	go m.Listen(":6666")

	m = vodka.New()
	m.Use(blimit.BodyLimitWithConfig(blimit.BodyLimitConfig{Skipper: skipper.DefaultSkipper, Limit: "4M"}))
	go m.Listen(":7777")
}
