package typ

import "sync"

type stringCache struct {
	once  sync.Once
	value string
}

func (c *stringCache) get(build func() string) string {
	c.once.Do(func() {
		c.value = build()
	})
	return c.value
}
