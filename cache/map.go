package cache

import "sync"

type Cache struct {
	m map[string]interface{}
	sync.RWMutex
}

func NewCache() *Cache {
	cache := new(Cache)
	cache.m = make(map[string]interface{})
	return cache
}

func (c *Cache) Put(k string, data interface{}) {
	c.Lock()
	c.m[k] = data
	c.Unlock()
}

func (c *Cache) Get(k string) (interface{}, bool) {
	c.RLock()
	defer c.RUnlock()
	data, ok := c.m[k]
	if ok {
		return data, ok
	} else {
		return nil, false
	}
}

func (c *Cache) Del(k string) {
	c.RLock()
	_, ok := c.m[k]
	if ok {
		c.RUnlock()
		c.Lock()
		delete(c.m, k)
		c.Unlock()
	} else {
		c.RUnlock()

	}
}
