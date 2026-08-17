package bauth_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/bauth"
	"testing"
)

func TestBasicAuth(t *testing.T) {
	m := vodka.New()
	m.Use(bauth.BasicAuth(func(username, password string) bool {
		if username == "inson" && password == "secret" {
			return true
		}
		return false
	}))

	go m.Listen(":9999")
}
