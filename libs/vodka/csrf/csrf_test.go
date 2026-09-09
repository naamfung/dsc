package csrf_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/csrf"
	"testing"
)

func TestCSRF(t *testing.T) {
	e := vodka.New()
	e.Use(csrf.CSRFWithConfig(csrf.CSRFConfig{
		TokenLookup: "header:" + vodka.HeaderXCSRFToken,
	}))
	go e.Listen(9000)
}
