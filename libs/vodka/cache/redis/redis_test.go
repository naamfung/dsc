package redis

import (
	"net"
	"testing"
	"time"

	"dsc/libs/vodka/cache"
)

func TestRedisCache(t *testing.T) {
	// 集成测试：无本地 Redis 服务时跳过，避免每次 go test ./... 报错干扰
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6379", 500*time.Millisecond)
	if err != nil {
		t.Skip("redis not available at 127.0.0.1:6379")
	}
	_ = conn.Close()

	c, err := cache.New(cache.Options{Adapter: "redis", AdapterConfig: `{"Addr":"127.0.0.1:6379"}`, Section: "test"})
	if err != nil {
		t.Fatal(err)
	}

	err = c.Set("da", "weisd", 300)
	if err != nil {
		t.Fatal(err)
	}

	res := ""
	err = c.Get("da", &res)
	if err != nil {
		t.Fatal(err)
	}

	if res != "weisd" {
		t.Fatal(res)
	}

	t.Log("ok")
	t.Log("test", res)

	err = c.Tags([]string{"dd"}).Set("da", "weisd", 300)
	if err != nil {
		t.Fatal(err)
	}
	res = ""
	err = c.Tags([]string{"dd"}).Get("da", &res)
	if err != nil {
		t.Fatal(err)
	}

	if res != "weisd" {
		t.Fatal("not weisd")
	}

	t.Log("ok")
	t.Log("dd", res)

	err = c.Tags([]string{"dsc/libs/vodka"}).Set("dsc/libs/vodka", "dsc/libs/vodka_contrib", 300)
	if err != nil {
		t.Fatal(err)
	}

	err = c.Tags([]string{"dsc/libs/vodka"}).Set("insion", "insionng", 300)
	if err != nil {
		t.Fatal(err)
	}

	res = ""
	err = c.Tags([]string{"dsc/libs/vodka"}).Get("dsc/libs/vodka", &res)
	if err != nil {
		t.Fatal(err)
	}

	if res != "dsc/libs/vodka_contrib" {
		t.Fatal("not vodka_contrib")
	}

	t.Log("ok")
	t.Log("dsc/libs/vodka", res)

	err = c.Tags([]string{"dsc/libs/vodka"}).Flush()
	if err != nil {
		t.Fatal(err)
	}

	res = ""
	c.Tags([]string{"dsc/libs/vodka"}).Get("dsc/libs/vodka", &res)
	if res != "" {
		t.Fatal("flush faield")
	}

	res = ""
	c.Tags([]string{"dsc/libs/vodka"}).Get("insion", &res)
	if res != "" {
		t.Fatal("flush faield")
	}

	res = ""
	err = c.Tags([]string{"dd"}).Get("da", &res)
	if err != nil {
		t.Fatal(err)
	}

	if res != "weisd" {
		t.Fatal("not weisd")
	}

	t.Log("ok")

	err = c.Flush()
	if err != nil {
		t.Fatal(err)
	}

	res = ""
	c.Get("da", &res)
	if res != "" {
		t.Fatal("flush failed")
	}

	t.Log("get dd da", res)

}
