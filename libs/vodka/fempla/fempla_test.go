package fempla_test

import (
	"testing"

	"dsc/libs/vodka"
	"dsc/libs/vodka/fempla"
)

func TestRender(t *testing.T) {
	m := vodka.New()
	m.SetRenderer(fempla.Renderor())
	m.Get("/", func() vodka.Handler {
		return func(self *vodka.Context) error {
			self.Set("title", "你好，世界")

			// render ./template/index.html file.
			return self.Render("index")
		}
	}())
}
