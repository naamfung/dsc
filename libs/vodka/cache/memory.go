package cache

import (
	"errors"
	"strconv"
	"sync"
	"time"

	"gopkg.in/vmihailenco/msgpack.v2"
)

var _ CacheStore = NewMemoryCacher()

// MemoryItem represents a memory cache item.
type MemoryItem struct {
	val     interface{}
	created int64
	expire  int64
}

// MemoryCacher represents a memory cache adapter implementation.
type MemoryCacher struct {
	lock     sync.RWMutex
	items    map[string]*MemoryItem
	interval int // GC interval.
}

// NewMemoryCacher creates and returns a new memory cacher.
func NewMemoryCacher() *MemoryCacher {
	return &MemoryCacher{items: make(map[string]*MemoryItem)}
}

// Set puts value into cache with key and expire time.
func (c *MemoryCacher) Set(key string, val interface{}, expire int64) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	b, e := msgpack.Marshal(val)
	if e != nil {
		return e
	}

	c.items[key] = &MemoryItem{
		val:     b,
		created: time.Now().Unix(),
		expire:  expire,
	}
	return nil
}

// put value into cache with key forever save
func (c *MemoryCacher) Forever(key string, val interface{}) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	b, e := msgpack.Marshal(val)
	if e != nil {
		return e
	}

	c.items[key] = &MemoryItem{
		val:     b,
		created: time.Now().Unix(),
		expire:  0,
	}

	return nil

}

// update expire time
func (c *MemoryCacher) Touch(key string, expire int64) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	item, ok := c.items[key]
	if !ok {
		return errors.New("key not exist")
	}

	item.created = time.Now().Unix()
	item.expire = expire

	c.items[key] = item

	return nil

}

// Get gets cached value by given key.
func (c *MemoryCacher) Get(key string, _val interface{}) error {
	c.lock.RLock()
	defer c.lock.RUnlock()

	item, ok := c.items[key]
	if !ok {
		return errors.New("item not exist")
	}
	if item.expire > 0 &&
		(time.Now().Unix()-item.created) >= item.expire {
		go c.Delete(key)
		return errors.New("item has expire")
	}

	b, _ := item.val.([]byte)
	return msgpack.Unmarshal(b, _val)
}

// Delete deletes cached value by given key.
func (c *MemoryCacher) Delete(key string) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.items, key)
	return nil
}

// Incr increases cached int-type value by given key as a counter.
func (c *MemoryCacher) Incr(key string) (int64, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	item, ok := c.items[key]
	if !ok {
		return 0, errors.New("key not exist")
	}
	// 使用正确的类型断言，val应该是[]byte类型
	b, ok := item.val.([]byte)
	if !ok {
		return 0, errors.New("value is not []byte")
	}
	// 解析为int64
	i, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, errors.New("value is not int64")
	}
	// 增加并重新编码
	newVal := i + 1
	newValBytes := []byte(strconv.FormatInt(newVal, 10))
	item.val = newValBytes
	return newVal, nil
}

// Decr decreases cached int-type value by given key as a counter.
func (c *MemoryCacher) Decr(key string) (int64, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	item, ok := c.items[key]
	if !ok {
		return 0, errors.New("key not exist")
	}
	// 使用正确的类型断言，val应该是[]byte类型
	b, ok := item.val.([]byte)
	if !ok {
		return 0, errors.New("value is not []byte")
	}
	// 解析为int64
	i, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return 0, errors.New("value is not int64")
	}
	// 减少并重新编码
	newVal := i - 1
	newValBytes := []byte(strconv.FormatInt(newVal, 10))
	item.val = newValBytes
	return newVal, nil
}

// IsExist returns true if cached value exists.
func (c *MemoryCacher) IsExist(key string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	_, ok := c.items[key]
	return ok
}

// Flush deletes all cached data.
func (c *MemoryCacher) Flush() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.items = make(map[string]*MemoryItem)
	return nil
}

func (c *MemoryCacher) checkExpiration(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	item, ok := c.items[key]
	if !ok {
		return
	}

	if (time.Now().Unix() - item.created) >= item.expire {
		delete(c.items, key)
	}
}

func (c *MemoryCacher) startGC() {
	if c.interval < 1 {
		return
	}

	if c.items != nil {
		c.lock.RLock()
		keys := make([]string, 0, len(c.items))
		for key := range c.items {
			keys = append(keys, key)
		}
		c.lock.RUnlock()

		for _, key := range keys {
			c.checkExpiration(key)
		}
	}

	time.AfterFunc(time.Duration(c.interval)*time.Second, func() { c.startGC() })
}

// StartAndGC starts GC routine based on config string settings.
func (c *MemoryCacher) StartAndGC(opt Options) error {
	c.interval = opt.Interval
	go c.startGC()
	return nil
}

func init() {
	Register("memory", NewMemoryCacher())
}
