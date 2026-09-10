package main

import (
	"fmt"

	"dsc/libs/vodka"
	//"dsc/libs/vodka/fault"
	"dsc/libs/vodka/logger"
	"dsc/libs/vodka/pongor"
	"dsc/libs/vodka/recover"
	"dsc/libs/vodka/static"
	//"log"
)

func main() {
	v := vodka.New()
	//v.Use(fault.Recovery(log.Printf))
	v.Use(
		recover.Recover(),
		logger.Logger(),
		static.Static("static"),
	)

	v.SetRenderer(pongor.Renderor())
	v.Get("/", func(self *vodka.Context) error {

		if self.Request.Method == vodka.GET {
			fmt.Println("Request Method Is [GET]!")
		} else {
			fmt.Println("Request Method Is Not [GET]!")
		}

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
