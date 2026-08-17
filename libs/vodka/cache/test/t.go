package main

import (
	"fmt"

	"dsc/libs/vodka"
	"dsc/libs/vodka/cache"
	_ "dsc/libs/vodka/cache/redis"
)

func main() {

	v := vodka.New()
	v.Use(cache.Cacher(cache.Options{Adapter: "redis", AdapterConfig: `{"Addr":"127.0.0.1:6379"}`, Section: "test", Interval: 5}))

	v.Get("/cache/put/", func(self *vodka.Context) error {
		err := cache.Store(self).Set("name", "dsc/libs/vodka", 60)
		if err != nil {
			return err
		}

		return self.String("store okay")
	})

	v.Get("/cache/get/", func(self *vodka.Context) error {
		var name string
		cache.Store(self).Get("name", &name)

		return self.String(fmt.Sprintf("get name %s", name))
	})

	v.Listen(":7891")
}
