package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCache_PutGet(t *testing.T) {
	cache := make(Cache)
	key := "expr"
	point := cfg.Point(1)

	cache.Put(key, point, typ.String)

	got, ok := cache.Get(key, point)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != typ.String {
		t.Errorf("expected String, got %v", got)
	}
}

func TestCache_GetMiss(t *testing.T) {
	cache := make(Cache)
	_, ok := cache.Get("missing", cfg.Point(1))
	if ok {
		t.Error("expected cache miss")
	}
}

func TestCache_DifferentPoints(t *testing.T) {
	cache := make(Cache)
	key := "expr"

	cache.Put(key, cfg.Point(1), typ.String)
	cache.Put(key, cfg.Point(2), typ.Number)

	t1, _ := cache.Get(key, cfg.Point(1))
	t2, _ := cache.Get(key, cfg.Point(2))

	if t1 != typ.String {
		t.Errorf("point 1: expected String, got %v", t1)
	}
	if t2 != typ.Number {
		t.Errorf("point 2: expected Number, got %v", t2)
	}
}

func TestCache_PutNilCache(t *testing.T) {
	var cache Cache
	cache.Put("key", cfg.Point(1), typ.String) // should not panic
}
