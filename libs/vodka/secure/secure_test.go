package secure_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/secure"
	"testing"
)

func TestSecure(t *testing.T) {
	m := vodka.New()
	m.Use(secure.Secure())
	go m.Listen(":8000")

	m = vodka.New()
	m.Use(secure.SecureWithConfig(secure.SecureConfig{
		XSSProtection:         "",
		ContentTypeNosniff:    "",
		XFrameOptions:         "",
		HSTSMaxAge:            3600,
		ContentSecurityPolicy: "default-src 'self'",
	}))
	go m.Listen(":9000")

}
