package core

import (
	"context"
	"testing"
)

func TestLSPDiagnosticsSetGet(t *testing.T) {
	client := NewLSPClient("go", "/tmp", "gopls")
	ctx := context.Background()

	diags := []LSPDiagnostic{
		{Range: LSPRange{Start: LSPPosition{Line: 5, Character: 10}, End: LSPPosition{Line: 5, Character: 15}},
			Severity: LSPSeverityError, Source: "gopls", Message: "undefined: foo"},
		{Range: LSPRange{Start: LSPPosition{Line: 10, Character: 0}, End: LSPPosition{Line: 10, Character: 5}},
			Severity: LSPSeverityWarning, Source: "gopls", Message: "unused variable: bar"},
	}
	client.SetDiagnostics("main.go", diags)

	got, err := client.GetDiagnostics(ctx, "main.go")
	if err != nil {
		t.Fatalf("GetDiagnostics: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(got))
	}
	if got[0].Message != "undefined: foo" {
		t.Errorf("diag 0 message = %q, want 'undefined: foo'", got[0].Message)
	}
}

func TestLSPGetAllDiagnostics(t *testing.T) {
	client := NewLSPClient("go", "/tmp", "gopls")
	ctx := context.Background()

	client.SetDiagnostics("a.go", []LSPDiagnostic{{Severity: LSPSeverityError, Message: "err1"}})
	client.SetDiagnostics("b.go", []LSPDiagnostic{{Severity: LSPSeverityWarning, Message: "warn1"}})

	all, err := client.GetAllDiagnostics(ctx)
	if err != nil {
		t.Fatalf("GetAllDiagnostics: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 files, got %d", len(all))
	}
}

func TestLSPFormatDiagnostics(t *testing.T) {
	diags := []LSPDiagnostic{
		{Range: LSPRange{Start: LSPPosition{Line: 4, Character: 9}, End: LSPPosition{Line: 4, Character: 14}},
			Severity: LSPSeverityError, Source: "gopls", Message: "undefined: foo"},
		{Range: LSPRange{Start: LSPPosition{Line: 9, Character: 0}, End: LSPPosition{Line: 9, Character: 5}},
			Severity: LSPSeverityWarning, Message: "unused: bar"},
	}
	formatted := FormatDiagnostics("main.go", diags)
	if formatted == "" {
		t.Error("formatted should not be empty")
	}
	if !containsStr(formatted, "ERROR") {
		t.Error("should contain ERROR severity")
	}
	if !containsStr(formatted, "WARN") {
		t.Error("should contain WARN severity")
	}
	if !containsStr(formatted, "undefined: foo") {
		t.Error("should contain error message")
	}
	if !containsStr(formatted, "Line 5") {
		t.Error("should contain line number (1-based: 4+1=5)")
	}
}

func TestLSPFormatDiagnosticsEmpty(t *testing.T) {
	formatted := FormatDiagnostics("clean.go", nil)
	if !containsStr(formatted, "No diagnostics") {
		t.Error("should say 'No diagnostics' for empty")
	}
}

func TestLSPSeverityLabel(t *testing.T) {
	cases := []struct {
		severity int
		want     string
	}{
		{LSPSeverityError, "ERROR"},
		{LSPSeverityWarning, "WARN"},
		{LSPSeverityInfo, "INFO"},
		{LSPSeverityHint, "HINT"},
		{99, "?"},
	}
	for _, c := range cases {
		if got := severityLabel(c.severity); got != c.want {
			t.Errorf("severityLabel(%d) = %q, want %q", c.severity, got, c.want)
		}
	}
}

func TestLSPToolExecute(t *testing.T) {
	client := NewLSPClient("go", "/tmp", "gopls")
	client.SetDiagnostics("main.go", []LSPDiagnostic{
		{Severity: LSPSeverityError, Message: "syntax error"},
	})
	tool := NewLSPTool(client)
	ctx := context.Background()

	// 指定文件
	result, err := tool.Execute(ctx, []byte(`{"file_path":"main.go"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !containsStr(result, "syntax error") {
		t.Errorf("result should contain 'syntax error', got %q", result)
	}

	// 所有文件
	result2, err := tool.Execute(ctx, []byte(`{}`))
	if err != nil {
		t.Fatalf("Execute all: %v", err)
	}
	if !containsStr(result2, "main.go") {
		t.Errorf("result should contain 'main.go', got %q", result2)
	}
}
