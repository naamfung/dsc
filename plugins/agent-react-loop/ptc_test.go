package main

import (
	"strings"
	"testing"

	"dsc/proto"
)

func TestPTCEnabled(t *testing.T) {
	t.Setenv("DSC_MODE", "")
	cases := map[string]bool{
		"":     false,
		"0":    false,
		"off":  false,
		"1":    true,
		"true": true,
		"on":   true,
		"ptc":  true,
		"PTC":  true,
		" 1 ":  true,
	}
	for env, want := range cases {
		t.Setenv("DSC_PTC", env)
		if got := ptcEnabled(); got != want {
			t.Fatalf("DSC_PTC=%q -> ptcEnabled=%v, want %v", env, got, want)
		}
	}
}

// TestPTCEnabledViaMode 验证处于 ptc preset（DSC_MODE=ptc）时，即使未单独设 DSC_PTC 也开启 PTC。
func TestPTCEnabledViaMode(t *testing.T) {
	t.Setenv("DSC_PTC", "")
	for _, mode := range []string{"ptc", "PTC", " ptc "} {
		t.Setenv("DSC_MODE", mode)
		if got := ptcEnabled(); !got {
			t.Fatalf("DSC_MODE=%q -> ptcEnabled=%v, want true", mode, got)
		}
	}
	t.Setenv("DSC_MODE", "standard")
	if got := ptcEnabled(); got {
		t.Fatalf("DSC_MODE=standard -> ptcEnabled=%v, want false", got)
	}
}

func TestOneLinePrompt(t *testing.T) {
	got := oneLinePrompt("  first line\nsecond line\n\nthird  ")
	if got != "first line second line third" {
		t.Fatalf("oneLinePrompt=%q", got)
	}
}

func TestFormatPTCTools(t *testing.T) {
	tools := []*proto.Tool{
		{Name: "run_code", Description: "execute a Lua program"},
		{Name: "subagent", Description: "delegate a task\n2nd line"},
		{Name: "shell", Description: strings.Repeat("x", 200)},
	}
	s := formatPTCTools(tools)
	if !strings.Contains(s, "run_code SDK") {
		t.Fatalf("missing SDK header")
	}
	if strings.Contains(s, "\n- run_code:") {
		t.Fatalf("run_code itself should be excluded from its own SDK")
	}
	if !strings.Contains(s, "\n- subagent: delegate a task 2nd line") {
		t.Fatalf("multi-line description not flattened:\n%s", s)
	}
	if !strings.Contains(s, "...") {
		t.Fatalf("long description not truncated:\n%s", s)
	}
}
