Session
==============

The session package is a vodka session manager. It can use many session providers.

## How to install?

	go get garpress/library/vodka/session


## What providers are supported?

As of now this session manager support memory, file and Redis .


## How to use it?

First you must import it

	import (
		"garpress/library/vodka/session"
	)


* Use **memory** as provider:

        session.Options{"memory", `{"cookieName":"iVodkaSessionID","gcLifetime":3600}`}

* Use **file** as provider, the last param is the path where you want file to be stored:

	    session.Options{"file", `{"cookieName":"iVodkaSessionID","gcLifetime":3600,"providerConfig":"./data/session"}`}

* Use **Redis** as provider, the last param is the Redis conn address,poolsize,password:

		session.Options{"redis", `{"cookieName":"iVodkaSessionID","gcLifetime":3600,"providerConfig":"127.0.0.1:6379,100,vodka"}`}

* Use **Cookie** as provider:

		session.Options{"cookie", `{"cookieName":"iVodkaSessionID","enableSetCookie":false,"gcLifetime":3600,"providerConfig":"{\"cookieName\":\"iVodkaSessionID\",\"securityKey\":\"garpress/library/vodkacookiehashkey\"}"}`}


Finally in the code you can use it like this

```go
package main

import (
	"garpress/library/vodka"
	"garpress/library/vodka/recover"
	"garpress/library/vodka/session"
	//_ "garpress/library/vodka/session/redis"
	"log"
)

func main() {

	v := vodka.New()
	v.Use(recover.Recover())
	v.Use(session.Sessioner(session.Options{"file", `{"cookieName":"iVodkaSessionID","gcLifetime":3600,"providerConfig":"./data/session"}`}))
	//v.Use(session.Sessioner(session.Options{"redis", `{"cookieName":"iVodkaSessionID","gcLifetime":3600,"providerConfig":"127.0.0.1:6379"}`}))

	v.Get("/get", func(self *vodka.Context) error {
		value := "nil"
		valueIf := self.Session.Get("key")
		if valueIf != nil {
			value = valueIf.(string)
		}

		return self.String(value)

	})

	v.Get("/set", func(self *vodka.Context) error {

		val := self.QueryParam("v")
		if len(val) == 0 {
			val = "value"
		}

		err := self.Session.Set("key", val)
		if err != nil {
			log.Printf("sess.set %v \n", err)
		}
		return self.String("ok")
	})

	v.Listen(":9000")
}

```


## How to write own provider?

When you develop a web app, maybe you want to write own provider because you must meet the requirements.

Writing a provider is easy. You only need to define two struct types
(Session and Provider), which satisfy the interface definition.
Maybe you will find the **memory** provider is a good example.

	type SessionStore interface {
		Set(key, value interface{}) error     //set session value
		Get(key interface{}) interface{}      //get session value
		Delete(key interface{}) error         //delete session value
		ID() string                    //back current sessionID
		Release(ctx *vodka.Context) error // release the resource & save data to provider & return the data
		Flush() error                         //delete all data
	}

	type Provider interface {
		Init(gcLifetime int64, config string) error
		Read(sid string) (vodka.RawStore, error)
		Exist(sid string) bool
		Regenerate(oldsid, sid string) (vodka.RawStore, error)
		Destroy(sid string) error
		Count() int //get all active session
		GC()
	}


## LICENSE

MIT License
