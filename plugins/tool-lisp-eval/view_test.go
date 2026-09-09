package main

import (
	"encoding/json"
	"testing"

	"dsc-sdk"
)

// TestLispEvalView lisp_eval 视图：纯文本，尾部耗时拆成头部徽标。
func TestLispEvalView(t *testing.T) {
	view, _ := lispEvalView("3\n\n[耗時 2ms]")
	var v dsc.View
	if err := json.Unmarshal(view, &v); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v.Kind != "plain" || v.Title != "Lisp" || v.Badge == nil || v.Badge.Text != "耗时 2ms" || v.Badge.Tone != "gray" {
		t.Fatalf("view 头 = %+v", v)
	}
	if v.Body != "3" {
		t.Fatalf("body = %q, want 3", v.Body)
	}

	// 无耗时尾缀：无徽标、正文原样
	view, _ = lispEvalView("(+ 1 2)")
	var v2 dsc.View
	if err := json.Unmarshal(view, &v2); err != nil {
		t.Fatalf("视图 JSON 非法: %v", err)
	}
	if v2.Badge != nil || v2.Body != "(+ 1 2)" {
		t.Fatalf("无耗时 view = %+v", v2)
	}
}
