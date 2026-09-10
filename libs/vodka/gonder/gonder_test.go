package gonder_test

import (
	"testing"

	"dsc/libs/vodka"
	"dsc/libs/vodka/gonder"
)

func TestRender(t *testing.T) {
	e := vodka.New()
	e.SetRenderer(gonder.Renderor())
	e.Get("/", func() vodka.Handler {
		return func(self *vodka.Context) error {
			self.Set("title", "你好，世界")
			// render ./template/index file.
			return self.Render("index")
		}
	}())
}
