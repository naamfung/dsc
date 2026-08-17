package jwt

import (
	"errors"
	"fmt"
	"dsc/libs/vodka"
	"dsc/libs/vodka/skipper"
	"reflect"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type (
	// JWTConfig defines the config for JWT middleware.
	JWTConfig struct {
		// Skipper defines a function to skip middleware.
		Skipper skipper.Skipper
		Expires time.Duration
		// Signing key to validate token.
		// Required.
		SigningKey interface{} `json:"signing_key"`

		// Signing method, used to check token signing method.
		// Optional. Default value HS256.
		SigningMethod string `json:"signing_method"`

		// Context key to store user information from the token into context.
		// Optional. Default value "token".
		ContextKey string `json:"context_key"`

		// Claims are extendable claims data defining token content.
		// Optional. Default value jwt.MapClaims
		Claims jwt.Claims

		// TokenLookup is a string in the form of "<source>:<name>" that is used
		// to extract token from the request.
		// Optional. Default value "header:Authorization".
		// Possible values:
		// - "header:<name>"
		// - "form:<name>"
		// - "query:<name>"
		// - "cookie:<name>"
		TokenLookup string `json:"token_lookup"`

		keyFunc jwt.Keyfunc
	}

	extractor func(*vodka.Context) (string, error)
)

var (
	Bearer = "Bearer" // 不能用const定义，需要允许被外部修改设定~
)

// Algorithims
const (
	AlgorithmHS256 = "HS256"
)

var (
	// DefaultJWTConfig is the default JWT auth middleware config.
	DefaultJWTConfig = JWTConfig{
		Skipper:       skipper.DefaultSkipper,
		Expires:       time.Hour,
		SigningMethod: AlgorithmHS256,
		ContextKey:    "token", //存储token信息的上下文键, 存储的值是*jwt.Token
		TokenLookup:   "header:" + vodka.HeaderAuthorization,
		Claims:        jwt.MapClaims{},
	}
)

// JWT returns a JSON Web Token (JWT) auth middleware.
//
// For valid token, it sets the user in context and calls next handler.
// For invalid token, it returns "401 - Unauthorized" error.
// For empty token, it returns "400 - Bad Request" error.
//
// See: https://jwt.io/introduction
// See `JWTConfig.TokenLookup`
func JWT(key string) vodka.Handler {
	c := DefaultJWTConfig
	c.SigningKey = []byte(key)
	return JWTWithConfig(c)
}

// JWTWithConfig returns a JWT auth middleware with config.
// See: `JWT()`.
func JWTWithConfig(config JWTConfig) vodka.Handler {

	// Defaults
	if config.Skipper == nil {
		config.Skipper = DefaultJWTConfig.Skipper
	}
	if config.Expires == 0 {
		config.Expires = DefaultJWTConfig.Expires
	}
	if config.SigningKey == nil {
		panic("jwt middleware requires signing key")
	}
	if config.SigningMethod == "" {
		config.SigningMethod = DefaultJWTConfig.SigningMethod
	}
	if config.ContextKey == "" {
		config.ContextKey = DefaultJWTConfig.ContextKey
	}
	if config.Claims == nil {
		config.Claims = DefaultJWTConfig.Claims
	}
	if config.TokenLookup == "" {
		config.TokenLookup = DefaultJWTConfig.TokenLookup
	}
	config.keyFunc = func(t *jwt.Token) (interface{}, error) {
		// Check the signing method
		if t.Method.Alg() != config.SigningMethod {
			return nil, fmt.Errorf("unexpected jwt signing method=%v", t.Header["alg"])
		}
		return config.SigningKey, nil
	}

	// Initialize
	parts := strings.Split(config.TokenLookup, ":")
	extractor := fromHeader(parts[1])
	switch parts[0] {
	case "form":
		extractor = fromForm(parts[1])
	case "query":
		extractor = fromQuery(parts[1])
	case "param":
		extractor = fromParam(parts[1])
	default:
		if parts[0] == "cookie" {
			extractor = fromCookie(parts[1])
		}
	}

	return func(c *vodka.Context) error {
		if config.Skipper(c) {
			return c.Next()
		}

		auth, err := extractor(c)
		if err != nil {
			// 当缺少或无效令牌时，返回401 Unauthorized而不是400 Bad Request
			return c.Break(vodka.StatusUnauthorized, vodka.ErrUnauthorized)
		}

		token := new(jwt.Token)

		if _, ok := config.Claims.(jwt.MapClaims); ok {
			token, err = jwt.Parse(auth, config.keyFunc)
		} else {
			if claims, okay := reflect.ValueOf(config.Claims).Interface().(jwt.Claims); okay {
				token, err = jwt.ParseWithClaims(auth, claims, config.keyFunc)
			}
		}
		if err == nil && token.Valid {
			// Store user information from token into context.
			c.Set(config.ContextKey, token)
			return c.Next()
		}

		return c.Break(vodka.StatusUnauthorized, vodka.ErrUnauthorized)
	}

}

func NewMapClaims() jwt.MapClaims {
	return make(jwt.MapClaims)
}

func GetMapClaims(self *vodka.Context, contextKey ...string) jwt.MapClaims {
	var key string
	if len(contextKey) == 0 {
		key = DefaultJWTConfig.ContextKey
	} else {
		key = contextKey[0]
	}

	if token, okay := self.Get(key).(*jwt.Token); okay {
		if claims, okay := token.Claims.(jwt.MapClaims); okay {
			return claims
		}
	}

	return nil
}

func GetMapClaimsFromToken(token *jwt.Token) jwt.MapClaims {
	if claims, okay := token.Claims.(jwt.MapClaims); okay {
		return claims
	}
	return nil
}

func ToToken(tokenInterface interface{}) (*jwt.Token, error) {
	token, okay := tokenInterface.(*jwt.Token)
	if !okay {
		return nil, errors.New("invalid token type")
	}
	return token, nil
}

func NewToken(alg string, claims jwt.MapClaims) *jwt.Token {

	token := new(jwt.Token)

	switch alg {
	case "HS256":
		token = jwt.New(jwt.SigningMethodHS256)
	case "HS512":
		token = jwt.New(jwt.SigningMethodHS512)
	case "RS256":
		token = jwt.New(jwt.SigningMethodRS256)
	case "RS512":
		token = jwt.New(jwt.SigningMethodRS512)
	case "ES256":
		token = jwt.New(jwt.SigningMethodES256)
	case "ES512":
		token = jwt.New(jwt.SigningMethodES512)
	default:
		token = jwt.New(jwt.SigningMethodHS256)
	}

	// Set a header and a claim
	token.Header["typ"] = "JWT"
	if claims["exp"] == nil {
		claims["exp"] = time.Now().Add(DefaultJWTConfig.Expires).Unix()
	}
	// Set claims
	token.Claims = claims

	return token

}

func NewTokenString(secret string, alg string, claims jwt.MapClaims) (string, error) {
	return NewToken(alg, claims).SignedString([]byte(secret))
}

// fromHeader returns a `extractor` that extracts token from request header.
func fromHeader(header string) extractor {
	return func(c *vodka.Context) (string, error) {
		auth := c.RequestHeader(header)
		l := len(Bearer)
		if len(auth) > l+1 && auth[:l] == Bearer {
			return auth[l+1:], nil
		}
		return "", errors.New("empty or invalid jwt in request header")
	}
}

// fromQuery returns a `extractor` that extracts token from query string.
func fromQuery(name string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.Query(name)
		if token == "" {
			return "", errors.New("empty jwt in query string")
		}
		return token, nil
	}
}

func fromParam(name string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.Param(name).String()
		if token == "" {
			return "", errors.New("empty jwt in param string")
		}
		return token, nil
	}
}

func fromForm(name string) func(c *vodka.Context) (string, error) {
	return func(c *vodka.Context) (string, error) {
		token := c.FormValue(name)
		if token == "" {
			return "", errors.New("empty jwt in form value")
		}
		return token, nil
	}
}

// fromCookie returns a `extractor` that extracts token from named cookie.
func fromCookie(name string) extractor {
	return func(c *vodka.Context) (string, error) {
		cookie, err := c.GetCookie(name)
		if err != nil || cookie == nil {
			return "", fmt.Errorf("empty jwt in cookie:%v", err)
		}
		return cookie.Value, nil
	}
}
