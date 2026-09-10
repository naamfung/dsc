package cache

import (
	"dsc/libs/vodka"
)

const VodkaCacheStoreKey = "VodkaCacheStore"

func Store(value interface{}) Cache {
	var cacher Cache
	var okay bool
	switch v := value.(type) {
	case *vodka.Context:
		if cacher, okay = v.Get(VodkaCacheStoreKey).(Cache); !okay {
			panic("Cacher not found, forget to Use Middleware ?")
		}
	default:
		panic("unknown Context")
	}

	if cacher == nil {
		panic("cache context not found")
	}

	return cacher
}

func Cacher(opt ...Options) vodka.Handler {
	var option Options
	if len(opt) > 0 {
		option = opt[0]
	} else {
		option = Options{Adapter: "memory"}
	}
	return func(self *vodka.Context) error {
		tagcache, err := New(option)
		if err != nil {
			return err
		}

		self.Set(VodkaCacheStoreKey, tagcache)

		if err = self.Next(); err != nil {
			return err
		}

		return nil
	}
}
