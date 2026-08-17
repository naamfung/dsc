# jwt for vodka

The jwt middleware for Macross Web Framework

## Requirements

vodka


## Getting Started

Create a `server.go` file with the following content:

```go
package main

import (
	"garpress/library/vodka"
	"garpress/library/vodka/jwt"
	"net/http"
)

func main() {
	m := vodka.New()


	m.Get("/", func(self *vodka.Context) error {
		data := self.Map("version", "1.0.0")
		return self.JSON(data, vodka.StatusOK)
	})

	// 给用户返回token之前请先密码验证用户身份
	m.Post("/signin/", func(self *vodka.Context) error {
		username := self.Args("username").String()
		password := self.Args("password").String()
		if (username == "jorsion") && (password == "PaSsworD") {
			claims := jwt.NewMapClaims()
			claims["address"] = "GD.GZ"
			tk, _ := jwt.NewToken("secret", "SigningMethodHS256", claims)

			return self.String(tk)
		}
		return vodka.ErrUnauthorized

	})

	g := m.Group("/jwt", jwt.JWT("secret"))
	g.Get("/say/", func(self *vodka.Context) error {
		return self.String("Hello, Vodka!")
	})

	m.Listen(":9000")
}

```

Now run the following command to start the Web server:

```
go run server.go
```

You should be able to access URLs such as `http://localhost:9000`.


