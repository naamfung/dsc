# Vodka

Package vodka is a high productive and modular web framework in Golang.

## Description

Vodka is a Go package that provides high performance and powerful HTTP vodka capabilities for Web applications.
Vodka is very fast, thanks to the radix tree data structure and the usage of `sync.Pool` 

It has the following features:

* middleware pipeline architecture, similar to that of the [Express framework](http://expressjs.com).
* extremely fast request vodka with zero dynamic memory allocation (the performance is comparable to that of [httprouter](https://github.com/julienschmidt/httprouter) and
[gin](https://github.com/gin-gonic/gin))
* modular code organization through route grouping
* flexible URL path matching, supporting URL parameters and regular expressions
* URL creation according to the predefined routes
* compatible with `http.Handler` and `http.HandlerFunc`
* ready-to-use handlers sufficient for building RESTful APIs

If you are using [fasthttp](https://github.com/valyala/fasthttp), you may use a similar vodka package [macross](https://github.com/insionng/macross) which is adapted from Vodka.

## Requirements

Golang 1.23.1 or above.

## Installation

Run the following command to install the package:

```
git clone --depth=1 https://gitcode.com/jorsion/vodka.git
```

## Usage

1. In the vodka directory, run the following command to replace all the occurrences of "vodka" like with "garpress/library/vodka" in all the `.go` files of the package:

```
find . -name "*.go" -exec sed -i 's|"vodka|"garpress/library/vodka|g' {} +
```

2. Remove the `.git` directory and go modules file:

```
rm -rf .git go.mod go.sum
```

3. At the root of your project, create a `go.mod` file with the following commands:

```
go mod init yourproject
go mod tidy
```

or if you are using go modules:

```
go mod tidy
```

4. Add the following lines to your golang file:
```go
import (
	"garpress/library/vodka"
)
```

5. Run the following command to build the project:

```
go build
```


## Getting Started

Create a `server.go` file with the following content:

```go
package main

import (
	"vodka"
)

func main() {
	m := vodka.New()
	
	m.Get("/", func(self *vodka.Context) error {
		return self.String("Hello, Vodka")
	})

	m.Listen(9000)
}
```

Now run the following command to start the Web server:

```
go run server.go
```

You should be able to access URLs such as `http://localhost:9000`.


## Getting Started via JWT

```go
package main

import (
	"fmt"
	"vodka"
	"vodka/cors"
	"vodka/jwt"
	"vodka/logger"
	"vodka/recover"
	"time"
)

/*
curl -I -X GET http://localhost:9000/jwt/get/ -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySWQiOjEsImV4cCI6MTQ3OTQ4NDUzOH0.amQOtO0GESwLoevaGSoR55jCUqZ6vsIi9DPTkDh4tSk"
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
  0    26    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0HTTP/1.1 200 OK
Server: Vodka
Date: Fri, 18 Nov 2016 15:55:18 GMT
Content-Type: application/json; charset=utf-8
Content-Length: 26
Vary: Origin
Access-Control-Allow-Origin: *
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySWQiOjEsImV4cCI6MTQ3OTQ4NDU3OH0.KBTm7A3xqWmQ6NLfUecfowgszfKzwMrjO3k0gf8llc8
*/

func main() {
	m := vodka.New()
	m.Use(logger.Logger())
	m.Use(recover.Recover())
	m.Use(cors.CORS())

	m.Get("/", func(self *vodka.Context) error {
		fmt.Println(self.Request.Header)
		var data = map[string]interface{}{}
		data["version"] = "1.0.0"
		return self.JSON(data)
	})

	var secret = "secret"
	var exprires = time.Minute * 1
	// 给用户返回token之前请先密码验证用户身份
	m.Post("/signin/", func(self *vodka.Context) error {

		fmt.Println(self.Response.String())

		username := self.Args("username").String()
		password := self.Args("password").String()
		if (username == "insion") && (password == "PaSsworD") {
			claims := jwt.NewMapClaims()
			claims["UserId"] = 1
			claims["exp"] = time.Now().Add(exprires).Unix()

			tk, _ := jwt.NewTokenString(secret, "HS256", claims)

			var data = map[string]interface{}{}
			data["token"] = tk

			return self.JSON(data)
		}

		herr := new(vodka.HTTPError)
		herr.Message = "ErrUnauthorized"
		herr.Status = vodka.StatusUnauthorized
		return self.JSON(herr, vodka.StatusUnauthorized)

	})

	g := m.Group("/jwt", jwt.JWT(secret))
	// http://localhost:9000/jwt/get/
	g.Get("/get/", func(self *vodka.Context) error {

		var data = map[string]interface{}{}

		claims := jwt.GetMapClaims(self)
		jwtUserId := claims["UserId"].(float64)
		fmt.Println(jwtUserId)
		exp := int64(claims["exp"].(float64))
		exptime := time.Unix(exp, 0).Sub(time.Now())

		if (exptime > 0) && (exptime < (exprires / 3)) {
			fmt.Println("exptime will be expires")
			claims["UserId"] = 1
			claims["exp"] = time.Now().Add(exprires).Unix()

			token := jwt.NewToken("HS256", claims)
			tokenString, _ := token.SignedString([]byte(secret))

			self.Response.Header.Set(vodka.HeaderAccessControlExposeHeaders, "Authorization")
			self.Response.Header.Set("Authorization", jwt.Bearer+" "+tokenString)
			self.Set(jwt.DefaultJWTConfig.ContextKey, token)
		}

		data["value"] = "Hello, Vodka"
		return self.JSON(data)
	})

	m.Listen(9000)
}
```


## Getting Started via Session

```go
package main

import (
	"vodka"
	"vodka/recover"
	"vodka/session"
	_ "vodka/session/redis"
	"log"
)

func main() {

	v := vodka.New()
	v.Use(recover.Recover())
	//v.Use(session.Sessioner(session.Options{"file", `{"cookieName":"VodkaSessionId","gcLifetime":3600,"providerConfig":"./data/session"}`}))
	v.Use(session.Sessioner(session.Options{"redis", `{"cookieName":"VodkaSessionId","gcLifetime":3600,"providerConfig":"127.0.0.1:6379"}`}))

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
		return self.String("okay")
	})

	v.Listen(7777)
}

```

## Getting Started via i18n

```go
package main

import (
	"fmt"
	"vodka"
	"vodka/i18n"
)

func main() {
	m := vodka.New()
	m.Use(i18n.I18n(i18n.Options{
		Directory:   "locale",
		DefaultLang: "zh-CN",
		Langs:       []string{"en-US", "zh-CN"},
		Names:       []string{"English", "简体中文"},
		Redirect:    true,
	}))

	m.Get("/", func(self *vodka.Context) error {
		fmt.Println("Header>", self.Request.Header.String())
		return self.String("current language is " + self.Language())
	})

	// Use in handler.
	m.Get("/trans", func(self *vodka.Context) error {
		fmt.Println("Header>", self.Request.Header.String())
		return self.String(fmt.Sprintf("hello %s", self.Tr("world")))
	})

	fmt.Println("Listen on 9999")
	m.Listen(9999)
}

```


## Getting Started via Go template

```go
package main

import (
	"vodka"
	"vodka/gonder"
	"vodka/logger"
	"vodka/recover"
	"vodka/static"
)

func main() {
	v := vodka.New()
	v.Use(logger.Logger())
	v.Use(recover.Recover())
	v.SetRenderer(gonder.Renderor())
	v.Use(static.Static("static"))
	v.Get("/", func(self *vodka.Context) error {
		var data = make(map[string]interface{})
		data["name"] = "Insion Ng"
		self.SetStore(data)

		self.SetStore(map[string]interface{}{
			"title": "你好，世界",
			"oh":    "no",
		})
		self.Set("oh", "yes") //覆盖前面指定KEY
		return self.Render("index")
	})

	v.Listen(":9000")
}

```

templates/index.html
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<script src="/static/index.js" charset="utf-8"></script>
<title>{{ .title }}</title>
</head>
<body>
    <p>{{ .oh }}</p
    <p>{{ .name }}</p>
</body>
</html>

```


## Getting Started via Pongo template

```go
package main

import (
	"vodka"
	"vodka/logger"
	"vodka/pongor"
	"vodka/recover"
	"vodka/static"
)

func main() {
	v := vodka.New()
	v.Use(logger.Logger())
	v.Use(recover.Recover())
	v.SetRenderer(pongor.Renderor())
	v.Use(static.Static("static"))
	v.Get("/", func(self *vodka.Context) error {
		var data = make(map[string]interface{})
		data["name"] = "Insion Ng"
		self.SetStore(data)

		self.SetStore(map[string]interface{}{
			"title": "你好，世界",
			"oh":    "no",
		})
		self.Set("oh", "yes") //覆盖前面指定KEY
		return self.Render("index")
	})

	v.Listen(":9000")
}

```

templates/index.html
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<script src="/static/index.js" charset="utf-8"></script>
<title>{{ title }}</title>
</head>
<body>
    <p>{{ oh }}</p
    <p>{{ name }}</p>
</body>
</html>

```

## Getting Started via FastTemplate

```go
package main

import (
	"vodka"
	"vodka/fempla"
	"vodka/logger"
	"vodka/recover"
	"vodka/static"
)

func main() {

	v := vodka.New()
	v.Use(logger.Logger())
	v.Use(recover.Recover())
	v.SetRenderer(fempla.Renderor())
	v.Use(static.Static("static"))
	v.Get("/", func(self *vodka.Context) error {
		data := make(map[string]interface{})
		data["oh"] = "no"
		data["name"] = "Insion Ng"
		self.Set("title", "你好，世界")
		self.SetStore(data)
		self.Set("oh", "yes")
		return self.Render("index")
	})

	v.Listen(":9000")

}

```

templates/index.html
```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<script src="/static/index.js" charset="utf-8"></script>
<title>{{title}}</title>
</head>
<body>
    <p>
        {{oh}}
    </p>
    <p>
        {{name}}
    </p>
</body>
</html>

```


## Case

Below we describe how to create a simple REST API using Vodka.

Create a `server.go` file with the following content:

```go
package main

import (
	"log"
	"net/http"
	"vodka"
	"vodka/access"
	"vodka/slash"
	"vodka/content"
	"vodka/fault"
	"vodka/file"
)

func main() {
	m := vodka.New()

	m.Use(
		// all these handlers are shared by every route
		access.Logger(log.Printf),
		slash.RemoveTrailingSlash(),
		fault.Recovery(log.Printf),
	)

	// serve RESTful APIs
	api := m.Group("/api")
	api.Use(
		// these handlers are shared by the routes in the api group only
		content.TypeNegotiator(content.JSON, content.XML),
	)
	api.Get("/users", func(c *vodka.Context) error {
		return c.Write("user list")
	})
	api.Post("/users", func(c *vodka.Context) error {
		return c.Write("create a new user")
	})
	api.Put(`/users/<id:\d+>`, func(c *vodka.Context) error {
		return c.Write("update user " + c.Param("id"))
	})

	// serve index file
	m.Get("/", file.Content("ui/index.html"))
	// serve files under the "ui" subdirectory
	m.Get("/*", file.Server(file.PathMap{
		"/": "/ui/",
	}))

	m.Listen(8888)
}
```

Create an HTML file `ui/index.html` with any content.

Now run the following command to start the Web server:

```
go run server.go
```

You should be able to access URLs such as `http://localhost:8888`, `http://localhost:8888/api/users`.


### Routes

Vodka works by building a vodka table in a router and then dispatching HTTP requests to the matching handlers 
found in the vodka table. An intuitive illustration of a vodka table is as follows:


Routes              |  Handlers
--------------------|-----------------
`GET /users`        |  m1, m2, h1, ...
`POST /users`       |  m1, m2, h2, ...
`PUT /users/<id>`   |  m1, m2, h3, ...
`DELETE /users/<id>`|  m1, m2, h4, ...


For an incoming request `GET /users`, the first route would match and the handlers m1, m2, and h1 would be executed.
If the request is `PUT /users/123`, the third route would match and the corresponding handlers would be executed.
Note that the token `<id>` can match any number of non-slash characters and the matching part can be accessed as 
a path parameter value in the handlers.

**If an incoming request matches multiple routes in the table, the route added first to the table will take precedence.
All other matching routes will be ignored.**

The actual implementation of the vodka table uses a variant of the radix tree data structure, which makes the vodka
process as fast as working with a hash table, thanks to the inspiration from [httprouter](https://github.com/julienschmidt/httprouter).

To add a new route and its handlers to the vodka table, call the `To` method like the following:
  
```go
m := vodka.New()
m.To("GET", "/users", m1, m2, h1)
m.To("POST", "/users", m1, m2, h2)
```

You can also use shortcut methods, such as `Get`, `Post`, `Put`, etc., which are named after the HTTP method names:
 
```go
m.Get("/users", m1, m2, h1)
m.Post("/users", m1, m2, h2)
```

If you have multiple routes with the same URL path but different HTTP methods, like the above example, you can 
chain them together as follows,

```go
m.Get("/users", m1, m2, h1).Post(m1, m2, h2)
```

If you want to use the same set of handlers to handle the same URL path but different HTTP methods, you can take
the following shortcut:

```go
m.To("GET,POST", "/users", m1, m2, h)
```

A route may contain parameter tokens which are in the format of `<name:pattern>`, where `name` stands for the parameter
name, and `pattern` is a regular expression which the parameter value should match. A token `<name>` is equivalent
to `<name:[^/]*>`, i.e., it matches any number of non-slash characters. At the end of a route, an asterisk character
can be used to match any number of arbitrary characters. Below are some examples:

* `/users/<username>`: matches `/users/admin`
* `/users/accnt-<id:\d+>`: matches `/users/accnt-123`, but not `/users/accnt-admin`
* `/users/<username>/*`: matches `/users/admin/profile/address`

When a URL path matches a route, the matching parameters on the URL path can be accessed via `Context.Param()`:

```go
m := vodka.New()

m.Get("/users/<username>", func (c *vodka.Context) error {
	fmt.Fprintf(c.Response, "Name: %v", c.Param("username"))
	return nil
})
```


### Route Groups

Route group is a way of grouping together the routes which have the same route prefix. The routes in a group also
share the same handlers that are registered with the group via its `Use` method. For example,

```go
m := vodka.New()
api := m.Group("/api")
api.Use(m1, m2)
api.Get("/users", h1).Post(h2)
api.Put("/users/<id>", h3).Delete(h4)
```

The above `/api` route group establishes the following vodka table:


Routes                  |  Handlers
------------------------|-------------
`GET /api/users`        |  m1, m2, h1, ...
`POST /api/users`       |  m1, m2, h2, ...
`PUT /api/users/<id>`   |  m1, m2, h3, ...
`DELETE /api/users/<id>`|  m1, m2, h4, ...


As you can see, all these routes have the same route prefix `/api` and the handlers `m1` and `m2`. In other similar
vodka frameworks, the handlers registered with a route group are also called *middlewares*.

Route groups can be nested. That is, a route group can create a child group by calling the `Group()` method. The router
serves as the top level route group. A child group inherits the handlers registered with its parent group. For example, 

```go
m := vodka.New()
m.Use(m1)

api := m.Group("/api")
api.Use(m2)

users := api.Group("/users")
users.Use(m3)
users.Put("/<id>", h1)
```

Because the vodka serves as the parent of the `api` group which is the parent of the `users` group, 
the `PUT /api/users/<id>` route is associated with the handlers `m1`, `m2`, `m3`, and `h1`.


### Router

Router manages the vodka table and dispatches incoming requests to appropriate handlers. A router instance is created
by calling the `vodka.New()` method.

Because `Router` implements the `http.Handler` interface, it can be readily used to serve subtrees on existing Go servers.
For example,

```go
m := vodka.New()
m.Listen(9999)
```


### Handlers

A handler is a function with the signature `func(*vodka.Context) error`. A handler is executed by the router if
the incoming request URL path matches the route that the handler is associated with. Through the `vodka.Context` 
parameter, you can access the request information in handlers.

A route may be associated with multiple handlers. These handlers will be executed in the order that they are registered
to the route. The execution sequence can be terminated in the middle using one of the following two methods:

* A handler returns an error: the router will skip the rest of the handlers and handle the returned error.
* A handler calls `Context.Abort()`: the router will simply skip the rest of the handlers. There is no error to be handled.
 
A handler can call `Context.Next()` to explicitly execute the rest of the unexecuted handlers and take actions after
they finish execution. For example, a response compression handler may start the output buffer, call `Context.Next()`,
and then compress and send the output to response.


### Context

For each incoming request, a `vodka.Context` object is populated with the request information and passed through
the handlers that need to handle the request. Handlers can get the request information via `Context.Request` and
send a response back via `Context.Response`. The `Context.Param()` method allows handlers to access the URL path
parameters that match the current route.

Using `Context.Get()` and `Context.Set()`, handlers can share data between each other. For example, an authentication
handler can store the authenticated user identity by calling `Context.Set()`, and other handlers can retrieve back
the identity information by calling `Context.Get()`.


### Reading Request Data

Context provides a few shortcut methods to read query parameters. The `Context.Query()`  method returns
the named URL query parameter value; the `Context.PostForm()` method returns the named parameter value in the POST or
PUT body parameters; and the `Context.Form()` method returns the value from either POST/PUT or URL query parameters.

The `Context.Read()` method supports reading data from the request body and populating it into an object.
The method will check the `Content-Type` HTTP header and parse the body data as the corresponding format.
For example, if `Content-Type` is `application/json`, the request body will be parsed as JSON data.
The public fields in the object being populated will receive the parsed data if the data contains the same named fields.
For example,

```go
func foo(c *vodka.Context) error {
    data := &struct{
        A string
        B bool
    }{}

    // assume the body data is: {"A":"abc", "B":true}
    // data will be populated as: {A: "abc", B: true}
    if err := c.Read(&data); err != nil {
        return err
    }
}
```

By default, `Context` supports reading data that are in JSON, XML, form, and multipart-form data.
You may modify `vodka.DataReaders` to add support for other data formats.

Note that when the data is read as form data, you may use struct tag named `form` to customize
the name of the corresponding field in the form data. The form data reader also supports populating
data into embedded objects which are either named or anonymous.

### Writing Response Data

The `Context.Write()` method can be used to write data of arbitrary type to the response.
By default, if the data being written is neither a string nor a byte array, the method will
will call `fmt.Fprint()` to write the data into the response.

You can call `Context.SetWriter()` to replace the default data writer with a customized one.
For example, the `content.TypeNegotiator` will negotiate the content response type and set the data
writer with an appropriate one.

### Error Handling

A handler may return an error indicating some erroneous condition. Sometimes, a handler or the code it calls may cause
a panic. Both should be handled properly to ensure best user experience. It is recommended that you use 
the `fault.Recover` handler or a similar error handler to handle these errors.

If an error is not handled by any handler, the router will handle it by calling its `handleError()` method which
simply sets an appropriate HTTP status code and writes the error message to the response.

When an incoming request has no matching route, the router will call the handlers registered via the `Router.NotFound()`
method. All the handlers registered via `Router.Use()` will also be called in advance. By default, the following two
handlers are registered with `Router.NotFound()`:

* `vodka.MethodNotAllowedHandler`: a handler that sends an `Allow` HTTP header indicating the allowed HTTP methods for a requested URL
* `vodka.NotFoundHandler`: a handler triggering 404 HTTP error

## Serving Static Files

Static files can be served with the help of `file.Server` and `file.Content` handlers. The former serves files
under the specified directories, while the latter serves the content of a single file. For example,

```go
import (
	"vodka"
	"vodka/file"
)

m := vodka.New()

// serve index file
m.Get("/", file.Content("ui/index.html"))
// serve files under the "ui" subdirectory
m.Get("/*", file.Server(file.PathMap{
	"/": "/ui/",
}))
```

## Handlers

Vodka comes with a few commonly used handlers in its subpackages:

Handler name 					| Description
--------------------------------|--------------------------------------------
[access.Logger](https://pkg.go.dev/gitcode.com/jorsion/vodka/access) | records an entry for every incoming request
[auth.Basic](https://pkg.go.dev/gitcode.com/jorsion/vodka/auth) | provides authentication via HTTP Basic
[auth.Bearer](https://pkg.go.dev/gitcode.com/jorsion/vodka/auth) | provides authentication via HTTP Bearer
[auth.Query](https://pkg.go.dev/gitcode.com/jorsion/vodka/auth) | provides authentication via token-based query parameter
[auth.JWT](https://pkg.go.dev/gitcode.com/jorsion/vodka/auth) | provides JWT-based authentication
[content.TypeNegotiator](https://pkg.go.dev/gitcode.com/jorsion/vodka/content) | supports content negotiation by response types
[content.LanguageNegotiator](https://pkg.go.dev/gitcode.com/jorsion/vodka/content) | supports content negotiation by accepted languages
[cors.Handler](https://pkg.go.dev/gitcode.com/jorsion/vodka/cors) | implements the CORS (Cross Origin Resource Sharing) specification from the W3C
[fault.Recovery](https://pkg.go.dev/gitcode.com/jorsion/vodka/fault) | recovers from panics and handles errors returned by handlers
[fault.PanicHandler](https://pkg.go.dev/gitcode.com/jorsion/vodka/fault) | recovers from panics happened in the handlers
[fault.ErrorHandler](https://pkg.go.dev/gitcode.com/jorsion/vodka/fault) | handles errors returned by handlers by writing them in an appropriate format to the response
[file.Server](https://pkg.go.dev/gitcode.com/jorsion/vodka/file) | serves the files under the specified folder as response content
[file.Content](https://pkg.go.dev/gitcode.com/jorsion/vodka/file) | serves the content of the specified file as the response
[slash.Remover](https://pkg.go.dev/gitcode.com/jorsion/vodka/slash) | removes the trailing slashes from the request URL and redirects to the proper URL

The following code shows how these handlers may be used:

```go
import (
	"log"
	"net/http"
	"vodka"
	"vodka/access"
	"vodka/slash"
	"vodka/fault"
)

m := vodka.New()

m.Use(
	access.Logger(log.Printf),
	slash.Remover(http.StatusMovedPermanently),
	fault.Recovery(log.Printf),
)

...
```

### Reload Tool
Recommended to use the `air` tool to reload the server when the code changes.

```bash
	go install github.com/air-verse/air@latest
```

Then in your project directory, run:

```bash
	air
```

The air tool's documentation is in Chinese:

	https://github.com/air-verse/air/blob/master/README-zh_cn.md


### Third-party Handlers


The following third-party handlers are specifically designed for Vodka:

Handler name 					| Description
--------------------------------|--------------------------------------------
[jwt.JWT](https://github.com/vvv-v13/ozzo-jwt) | supports JWT Authorization


Vodka also provides adapters to support using third-party `http.HandlerFunc` or `http.Handler` handlers. 
For example,

```go
m := vodka.New()

// using http.HandlerFunc
m.Use(vodka.HTTPHandlerFunc(http.NotFound))

// using http.Handler
m.Use(vodka.HTTPHandler(http.NotFoundHandler))
```

### Contributes

Thanks to the macross, com, echo/vodka, iris, gin, beego, ozzo-routing, FastTemplate, Pongo2, Jwt-go. And all other Go package dependencies projects


### Recipes

- [Garpress](https://gitcode.com/jorsion/garpress/overview) Garpress,the cms project like wordpress
