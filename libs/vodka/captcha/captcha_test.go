package captcha

import (
	"fmt"
	"testing"

	"dsc/libs/vodka"
	"dsc/libs/vodka/cache"
	"dsc/libs/vodka/logger"
	"dsc/libs/vodka/pongor"

	. "github.com/smartystreets/goconvey/convey"
)

func Test_Version(t *testing.T) {
	Convey("Get version", t, func() {
		So(Version(), ShouldEqual, _VERSION)
	})
}

func Test_CaptchaMiddleware(t *testing.T) {
	Convey("Captcha middleware", t, func() {
		v := vodka.New()
		v.Use(logger.Logger())
		v.Use(cache.Cacher(cache.Options{Adapter: "memory"}))
		v.Use(Captchaer())
		v.SetRenderer(pongor.Renderor(pongor.Option{Directory: "test/template"}))

		v.Get("/", func(c *vodka.Context) error {
			if cpt := c.Get("Captcha"); cpt != nil {
				fmt.Println("Got:", cpt)
			} else {
				fmt.Println("Captcha is nil!")
			}

			c.Set("title", "你好，世界")
			return c.Render("index")
		})

		go v.Listen(":9988") // 启动服务器

	})
}
