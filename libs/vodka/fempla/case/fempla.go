package main

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/fempla"
	"dsc/libs/vodka/logger"
	"dsc/libs/vodka/recover"
	"dsc/libs/vodka/static"
)

func main() {

	v := vodka.New()
	v.Use(logger.Logger())
	v.Use(recover.Recover())
	v.SetRenderer(fempla.Renderor())
	v.Use(static.Static("static"))
	v.Get("/", func(self *vodka.Context) error {
		data := make(map[string]interface{})
		data["oh"] = "no"
		data["name"] = "Insion Ng"
		self.Set("title", "你好，世界")
		self.SetStore(data)
		self.Set("oh", "yes")
		return self.Render("index")
	})

	v.Listen(9000)

}
