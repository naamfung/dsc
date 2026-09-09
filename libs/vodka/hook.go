package vodka

import (
	"sync"

	"dsc/libs/vodka/prior"
)

var (
	// DefaultPriority 默认优先级为0值
	DefaultPriority float64
	callback        func([]byte) []byte
)

// NewPriorityQueue New PriorityQueue
func (m *Vodka) NewPriorityQueue() *prior.PriorityQueue {
	return prior.NewPriorityQueue()
}

func (m *Vodka) NewNode(key interface{}, v interface{}, priority float64) *prior.Node {
	return prior.NewNode(key, v, priority)
}

// NewPriorityQueue New PriorityQueue
func (c *Context) NewPriorityQueue() *prior.PriorityQueue {
	return c.vodka.NewPriorityQueue()
}

func (c *Context) NewNode(key interface{}, v interface{}, priority float64) *prior.Node {
	return prior.NewNode(key, v, priority)
}

// SetPriorityQueueWith c.vodka.QueuesMap[key] = c.NewPriorityQueue()
func (m *Vodka) SetPriorityQueueWith(key interface{}) *sync.Map {
	if m.QueuesMap == nil {
		m.QueuesMap = new(sync.Map)
	}
	m.QueuesMap.Store(key, m.NewPriorityQueue())
	return m.QueuesMap
}

// SetPriorityQueueWith c.vodka.QueuesMap[key] = c.NewPriorityQueue()
func (c *Context) SetPriorityQueueWith(key interface{}) *sync.Map {
	return c.vodka.SetPriorityQueueWith(key)
}

// RemoveFilterHook (m *Vodka) 删除过滤钩子
func (m *Vodka) RemoveFilterHook(key string) {
	if m.HasFilterHook(key) {
		m.FiltersMap.Delete(key)
	}
}

// RemoveFilterHook (c *Context) 删除过滤钩子
func (c *Context) RemoveFilterHook(key string, globals ...bool) {
	var global = false
	if len(globals) > 0 {
		global = globals[0]
	}
	if global {
		c.vodka.RemoveFilterHook(key)
	} else if c.HasFilterHook(key) {
		c.FiltersMap.Delete(key)
	}
}

// RemoveActionHook (m *Vodka) 删除动作钩子
func (m *Vodka) RemoveActionHook(key string) {
	if m.HasActionHook(key) {
		m.QueuesMap.Delete(key)
		m.FiltersMap.Delete(key)
	}
}

// RemoveActionHook (c *Context) 删除动作钩子
func (c *Context) RemoveActionHook(key string, globals ...bool) {

	var global = false
	if len(globals) > 0 {
		global = globals[0]
	}
	if global {
		c.vodka.RemoveActionHook(key)
	} else if c.HasActionHook(key) {
		c.vodka.QueuesMap.Delete(key)
		c.FiltersMap.Delete(key)
	}
}

// RemoveActionsHook (m *Vodka) 删除所有动作钩子
func (m *Vodka) RemoveActionsHook() {
	m.QueuesMap = nil
}

// RemoveActionsHook (c *Context) 删除所有动作钩子
func (c *Context) RemoveActionsHook() {
	c.vodka.RemoveActionsHook()
}

// HasFilterHook (m *Vodka) 是否有过滤钩子
func (m *Vodka) HasFilterHook(key string) bool {
	if _, okay := m.FiltersMap.Load(key); okay {
		return true
	}
	return false
}

// HasFilterHook (c *Context) 是否有过滤钩子
func (c *Context) HasFilterHook(key string, globals ...bool) bool {

	var global = false
	if len(globals) > 0 {
		global = globals[0]
	}
	if global {
		return c.vodka.HasFilterHook(key)
	}
	if c.FiltersMap != nil {
		if _, okay := c.FiltersMap.Load(key); okay {
			return true
		}
	}

	return false
}

// HasQueuesMap (m *Vodka) Has QueuesMap
func (m *Vodka) HasQueuesMap(key string) bool {
	if value, okay := m.QueuesMap.Load(key); okay {
		if pqueue, okay := value.(*prior.PriorityQueue); okay {
			if pqueue.Length() > 0 {
				return true
			}
		}
	}
	return false
}

// HasQueuesMap (c *Context) Has QueuesMap
func (c *Context) HasQueuesMap(key string) bool {
	return c.vodka.HasQueuesMap(key)
}

// HasActionHook (m *Vodka) 是否有动作钩子
func (m *Vodka) HasActionHook(key string) bool {
	if value, okay := m.QueuesMap.Load(key); okay {
		if _, okay := value.(*prior.PriorityQueue); okay {
			return true
		}
	}
	return false
}

// HasActionHook (c *Context) 是否有动作钩子
func (c *Context) HasActionHook(key string) bool {
	return c.vodka.HasActionHook(key)
}

// AddActionHook (m *Vodka) 增加动作钩子
func (m *Vodka) AddActionHook(key string, function func(), priorities ...float64) {
	m.AddFilterHook(key, func([]byte) []byte {
		function()
		return nil
	}, priorities...)
}

// AddActionHook (c *Context) 增加动作钩子
func (c *Context) AddActionHook(key string, function func(), priorities ...float64) {
	c.AddFilterHook(key, func([]byte) []byte {
		function()
		return nil
	}, priorities...)
}

// AddFilterHook (m *Vodka) 增加过滤钩子
func (m *Vodka) AddFilterHook(key string, function func([]byte) []byte, priorities ...float64) {

	if !m.HasQueuesMap(key) {
		m.SetPriorityQueueWith(key)
	}

	var priority float64
	if len(priorities) > 0 {
		priority = priorities[0]
	} else {
		priority = DefaultPriority
	}
	pq := m.NewPriorityQueue()
	pq.Push(prior.NewNode(key, function, priority))
	m.QueuesMap.Store(key, pq)

}

// AddFilterHook (c *Context) 增加过滤钩子
func (c *Context) AddFilterHook(key string, function func([]byte) []byte, priorities ...float64) {

	if !c.HasQueuesMap(key) {
		c.vodka.SetPriorityQueueWith(key)
	}

	var priority float64
	if len(priorities) > 0 {
		priority = priorities[0]
	} else {
		priority = DefaultPriority
	}

	pq := c.NewPriorityQueue()
	pq.Push(prior.NewNode(key, function, priority))
	c.vodka.QueuesMap.Store(key, pq)
}

// DoActionHook (m *Vodka) 动作钩子
func (m *Vodka) DoActionHook(key string) {
	m.DoFilterHook(key, nil)
}

// DoActionHook (c *Context) 动作钩子
func (c *Context) DoActionHook(key string, globals ...bool) {
	c.DoFilterHook(key, nil, globals...)
}

// DoFilterHook (m *Vodka) 执行过滤钩子
func (m *Vodka) DoFilterHook(key string, function func() []byte) []byte {
	if !m.HasQueuesMap(key) {
		m.AddFilterHook(key, func(b []byte) []byte {
			if function == nil {
				return b
			}
			return function()
		})
		return m.DoFilterHook(key, function)
	}
	if !m.HasFilterHook(key) {
		if m.FiltersMap == nil {
			m.FiltersMap = new(sync.Map) //make(map[string][]byte)
		}
	}

	if m.HasActionHook(key) {
		if queue, okay := m.QueuesMap.Load(key); okay {
			pq, okay := queue.(*prior.PriorityQueue)
			if !okay {
				return nil
			}
			for pq.Length() > 0 {
				if node := pq.Pop(); node != nil {
					if value := node.GetValue(); value != nil {
						if callback, okay = value.(func([]byte) []byte); !okay {
							return nil
						}
					} else {
						continue
					}
				} else {
					continue
				}

				if function != nil { //for Global Filter
					if value, okay := m.FiltersMap.Load(key); okay {
						if b, okay := value.([]byte); okay {
							m.FiltersMap.Store(key, callback(b))
						}
					} else {
						m.FiltersMap.Store(key, callback(function()))
					}
				} else { //for Global Action
					m.FiltersMap.Store(key, callback(nil))
				}

			}
		}
	}

	if value, okay := m.FiltersMap.Load(key); okay {
		if b, okay := value.([]byte); okay {
			return b
		}
	}
	return nil
}

// DoFilterHook (c *Context) 执行过滤钩子
func (c *Context) DoFilterHook(key string, function func() []byte, globals ...bool) []byte {
	var global = false
	if len(globals) > 0 {
		global = globals[0]
	}

	var filterBytes []byte

	if !c.HasQueuesMap(key) {
		c.AddFilterHook(key, func(b []byte) []byte {
			if function == nil {
				return b
			}
			return function()
		})
		return c.DoFilterHook(key, function, globals...)
	}

	if !c.HasFilterHook(key, global) {
		if global {
			if c.vodka.FiltersMap == nil {
				c.vodka.FiltersMap = new(sync.Map) //make(map[string][]byte)
			}
		} else {
			if c.FiltersMap == nil {
				c.FiltersMap = new(sync.Map) // make(map[string][]byte)
			}
		}
	}

	if c.HasActionHook(key) {
		if queue, okay := c.vodka.QueuesMap.Load(key); okay {
			pq, okay := queue.(*prior.PriorityQueue)
			if !okay {
				return nil
			}
			for pq.Length() > 0 {
				if node := pq.Pop(); node != nil {
					if value := node.GetValue(); value != nil {
						if callback, okay = value.(func([]byte) []byte); !okay {
							return nil
						}
					} else {
						continue
					}
				} else {
					continue
				}

				processMap := func(filterMap *sync.Map, key string, function func() []byte, callback func([]byte) []byte) []byte {
					if function != nil { // for Filter
						if value, okay := filterMap.Load(key); okay {
							if b, okay := value.([]byte); okay {
								filterMap.Store(key, callback(b))
							}
						} else {
							filterMap.Store(key, callback(function()))
						}
					} else { // for Action
						filterMap.Store(key, callback(nil))
					}

					if value, okay := filterMap.Load(key); okay {
						if b, okay := value.([]byte); okay {
							return b
						}
					}
					return nil
				}

				if global {
					filterBytes = processMap(c.vodka.FiltersMap, key, function, callback)
				} else {
					filterBytes = processMap(c.FiltersMap, key, function, callback)
				}
			}
		}
	}

	return filterBytes

}
