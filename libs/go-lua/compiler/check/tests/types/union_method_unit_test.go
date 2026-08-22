package types

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// 6) Method Call on Union

func TestUnionMethod_NarrowedMethodCall(t *testing.T) {
	source := `
		type A = {tag: "a", methodA: fun(self: self): string}
		type B = {tag: "b", methodB: fun(self: self): number}

		function process(r: A | B)
			if r.tag == "a" then
				local s: string = r:methodA()
			else
				local n: number = r:methodB()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for narrowed method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestUnionMethod_ElseBranchWrongMethod(t *testing.T) {
	source := `
		type A = {tag: "a", methodA: fun(self: self): string}
		type B = {tag: "b", methodB: fun(self: self): number}

		function process(r: A | B)
			if r.tag == "a" then
				local s: string = r:methodA()
			else
				local s: string = r:methodA()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error for wrong method in else branch")
	}
}

func TestUnionMethod_CommonMethod(t *testing.T) {
	source := `
		type A = {tag: "a", common: fun(self: self): string}
		type B = {tag: "b", common: fun(self: self): string}

		function process(r: A | B): string
			return r:common()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for common method, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestUnionMethod_BooleanNarrowing(t *testing.T) {
	source := `
		type Success = {ok: true, value: string, get: fun(self: self): string}
		type Failure = {ok: false, error: string, msg: fun(self: self): string}

		function process(r: Success | Failure)
			if r.ok then
				local v: string = r:get()
			else
				local e: string = r:msg()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for boolean narrowing method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
