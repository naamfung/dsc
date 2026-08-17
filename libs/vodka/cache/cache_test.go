package cache

import (
	"testing"
)

func Test_TagCache(t *testing.T) {

	c, err := New(Options{Adapter: "memory"})
	if err != nil {
		t.Fatal(err)
	}

	// base use
	err = c.Set("da", "weisd", 300)
	if err != nil {
		t.Fatal(err)
	}

	res := ""
	c.Get("da", &res)

	if res != "weisd" {
		t.Fatal("base put faield")
	}

	t.Log("ok")

	// use tags/namespace
	err = c.Tags([]string{"insion"}).Set("da", "weisd", 300)
	if err != nil {
		t.Fatal(err)
	}
	res = ""
	c.Tags([]string{"insion"}).Get("da", &res)

	if res != "weisd" {
		t.Fatal("tags put faield")
	}

	t.Log("ok")

	err = c.Tags([]string{"dsc/libs/vodka"}).Set("dsc/libs/vodka", "dsc/libs/vodka_contrib", 300)
	if err != nil {
		t.Fatal(err)
	}

	res = ""
	c.Tags([]string{"dsc/libs/vodka"}).Get("dsc/libs/vodka", &res)

	if res != "dsc/libs/vodka_contrib" {
		t.Fatal("not vodka_contrib")
	}

	t.Log("ok")

	// flush namespace
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
	c.Tags([]string{"dsc/libs/vodka"}).Get("bb", &res)
	if res != "" {
		t.Fatal("flush faield")
	}

	// still store in
	res = ""
	c.Tags([]string{"insion"}).Get("da", &res)
	if res != "weisd" {
		t.Fatal("where")
	}

	t.Log("ok")

}
