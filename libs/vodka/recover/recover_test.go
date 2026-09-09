package recover_test

import (
	"dsc/libs/vodka"
	"dsc/libs/vodka/recover"
	"testing"
)

func TestRecover(t *testing.T) {
	m := vodka.New()
	m.Use(recover.RecoverWithConfig(recover.RecoverConfig{
		StackSize: 1 << 10, // 1 KB
	}))
	go m.Listen(":8888")
}
