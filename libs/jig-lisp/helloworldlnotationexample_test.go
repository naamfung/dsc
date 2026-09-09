package lisp

import (
	"context"
	"fmt"
	"log"

	"github.com/jig/lisp/env"
	"github.com/jig/lisp/lib/core"
	. "github.com/jig/lisp/lnotation"
	"github.com/jig/lisp/types"
)

func ExampleL() {
	var (
		prn = S("prn")
		str = S("str")
	)
	sampleCode := L(prn, L(str, "hello", " ", "world!"))

	result, err := EVAL(context.TODO(), sampleCode, newTestEnv())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result)
	// Output: "hello world!"
	// <nil>
}

func newTestEnv() types.EnvType {
	newEnv := env.NewEnv()
	core.Load(newEnv)
	return newEnv
}
