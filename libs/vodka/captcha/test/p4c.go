package main

import (
	"fmt"
	"log"

	"dsc/libs/vodka"
	"dsc/libs/vodka/cache"
	"dsc/libs/vodka/captcha"
	"dsc/libs/vodka/logger"
	"dsc/libs/vodka/pongor"
	"dsc/libs/vodka/recover"
	"dsc/libs/vodka/session"
)

func main() {
	v := vodka.New()
	v.Use(logger.Logger())
	v.Use(recover.Recover())
	v.Use(cache.Cacher(cache.Options{Adapter: "memory"}))
	v.Use(captcha.Captchaer())
	v.Use(session.Sessioner())
	v.SetRenderer(pongor.Renderor(pongor.Option{Reload: true}))

	v.Get("/", func(self *vodka.Context) error {

		if cpt := self.Get("Captcha"); cpt != nil {
			fmt.Println("Got:", cpt)
		} else {
			fmt.Println("Captcha is nil!")
		}

		self.Set("title", "你好，世界")
		return self.Render("index")
	})

	v.Post("/verify/", func(self *vodka.Context) error {
		if cpt := self.Get("Captcha"); cpt != nil {
			if cpts := captcha.Store(self); !cpts.VerifyReq(self) {
				log.Println("Captcha is invalid!")
				self.Flash.Error("验证码无效！")
			} else {
				log.Println("Captcha is valid!")
				self.Flash.Error("验证码有效！")
			}
		}
		return self.Redirect("/")
	})

	v.Listen(":7891")
}
