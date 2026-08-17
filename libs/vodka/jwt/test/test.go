package main

import (
	"fmt"
	"dsc/libs/vodka"
	"dsc/libs/vodka/cors"
	"dsc/libs/vodka/jwt"
	"dsc/libs/vodka/logger"
	"dsc/libs/vodka/recover"
	"time"
)

/*
curl -I -X GET http://localhost:9000/jwt/get/ -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySWQiOjEsImV4cCI6MTQ3OTQ4NDUzOH0.amQOtO0GESwLoevaGSoR55jCUqZ6vsIi9DPTkDh4tSk"
  % Total    % Received % Xferd  Average Speed   Time    Time     Time  Current
                                 Dload  Upload   Total   Spent    Left  Speed
  0    26    0     0    0     0      0      0 --:--:-- --:--:-- --:--:--     0HTTP/1.1 200 OK
Server: vodka
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
		fmt.Println(self.Response.Header())
		var data = map[string]interface{}{}
		data["version"] = "1.0.0"
		return self.JSON(data)
	})

	var secret = "secret"
	var exprires = time.Minute * 1
	// 给用户返回token之前请先密码验证用户身份
	m.Post("/signin/", func(self *vodka.Context) error {

		fmt.Println(self.Response.Header())

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
			self.Response.Header().Set(vodka.HeaderAccessControlExposeHeaders, "Authorization")
			self.Response.Header().Set("Authorization", jwt.Bearer+" "+tokenString)
			self.Set(jwt.DefaultJWTConfig.ContextKey, token)
		}

		data["value"] = "Hello, vodka"
		return self.JSON(data)
	})

	m.Listen(":9000")
}
