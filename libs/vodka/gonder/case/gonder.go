package main

import (
	"dsc/libs/vodka"
	//"dsc/libs/vodka/access"
	//"dsc/libs/vodka/file"
	"dsc/libs/vodka/gonder"
	//"dsc/libs/vodka/logger"
	//"dsc/libs/vodka/recover"
	"dsc/libs/vodka/static"
	//"log"
)

func main() {
	v := vodka.New()

	//v.Use(recover.Recover())
	//v.Use(logger.Logger())
	//v.Use(access.Logger(log.Printf))
	v.Use(static.Static("static"))
	//v.Use(file.Server(file.PathMap{"/static": "/static"}))

	v.SetRenderer(gonder.Renderor(gonder.Option{
		DelimLeft:  "{{",
		DelimRight: "}}",
	}))

	v.Get("/", func(self *vodka.Context) error {
		var data = make(map[string]interface{})
		data["name"] = "Insion Ng"
		self.SetStore(data)

		self.SetStore(map[string]interface{}{
			"title": "你好，世界",
			"oh":    "no",
		})
		self.Set("oh", "yes") //覆盖前面指定KEY
		return self.Render("index")
	})

	v.Listen(":9000")
}
