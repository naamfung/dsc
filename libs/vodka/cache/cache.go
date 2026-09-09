// Copyright 2016~2017 Insionng
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.
package cache

import (
	"fmt"
)

const _VERSION = "0.1.0"

func Version() string {
	return _VERSION
}

var _ Cache = new(Engine)

// Cache is the interface that operates the cache data.
type CacheStore interface {
	// Set puts value into cache with key and expire time.
	Set(key string, val interface{}, timeout int64) error
	// Get gets cached value by given key.
	Get(key string, _val interface{}) error
	// Delete deletes cached value by given key.
	Delete(key string) error
	// Incr increases cached int-type value by given key as a counter.
	Incr(key string) (int64, error)
	// Decr decreases cached int-type value by given key as a counter.
	Decr(key string) (int64, error)
	// IsExist returns true if cached value exists.
	IsExist(key string) bool
	// Flush deletes all cached data.
	Flush() error
	// StartAndGC starts GC routine based on config string settings.
	StartAndGC(opt Options) error
	// update expire time
	Touch(key string, expire int64) error
}

type Cache interface {
	CacheStore
	Tags(tags []string) Cache
}

type Options struct {
	// Name of adapter. Default is "memory".
	Adapter string
	// Adapter configuration, it's corresponding to adapter.
	AdapterConfig string
	// GC interval time in seconds. Default is 60.
	Interval int
	// key prefix Default is ""
	Section string
}

func prepareOptions(options []Options) Options {
	var opt Options
	if len(options) > 0 {
		opt = options[0]
	}

	if len(opt.Adapter) == 0 {
		opt.Adapter = "memory"
	}
	if opt.Interval == 0 {
		opt.Interval = 60
	}

	return opt
}

func New(options ...Options) (Cache, error) {
	opt := prepareOptions(options)

	adapter, ok := adapters[opt.Adapter]
	if !ok {
		return nil, fmt.Errorf("cache: unknown adapter '%s'(forgot to import?)", opt.Adapter)
	}

	engine := &Engine{}
	engine.Opt = opt
	engine.store = adapter

	return engine, adapter.StartAndGC(opt)
}

type Engine struct {
	Opt   Options
	store CacheStore
}

func (e *Engine) Set(key string, val interface{}, timeout int64) error {
	return e.store.Set(key, val, timeout)
}

func (e *Engine) Get(key string, _val interface{}) error {
	return e.store.Get(key, _val)
}

func (e *Engine) Delete(key string) error {
	return e.store.Delete(key)
}

func (e *Engine) Incr(key string) (int64, error) {
	return e.store.Incr(key)
}

func (e *Engine) Decr(key string) (int64, error) {
	return e.store.Decr(key)
}

func (e *Engine) IsExist(key string) bool {
	return e.store.IsExist(key)
}

func (e *Engine) Flush() error {
	return e.store.Flush()
}

func (e *Engine) StartAndGC(opt Options) error {
	return e.store.StartAndGC(opt)
}

func (e *Engine) Touch(key string, expire int64) error {
	return e.store.Touch(key, expire)
}

func (e *Engine) Tags(tags []string) Cache {
	return NewTagCache(e.store, tags...)
}

var adapters = make(map[string]CacheStore)

// Register registers a adapter.
func Register(name string, adapter CacheStore) {
	if adapter == nil {
		panic("cache: cannot register adapter with nil value")
	}
	if _, dup := adapters[name]; dup {
		panic(fmt.Errorf("cache: cannot register adapter '%s' twice", name))
	}
	adapters[name] = adapter
}
