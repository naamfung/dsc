package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkflowToolRegistered(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	tool, ok := m.toolRegistry.Get("workflow")
	if !ok {
		t.Fatal("workflow tool should be registered")
	}
	if tool.Name() != "workflow" || tool.Description() == "" {
		t.Fatalf("workflow tool = %s", tool.Name())
	}
	if len(tool.ParametersSchema()) == 0 {
		t.Fatal("workflow tool should have a parameters schema")
	}
}

func TestWorkflowToolExecute(t *testing.T) {
	m := NewManager(&ManagerConfig{ExecDir: t.TempDir()})
	tool, _ := m.toolRegistry.Get("workflow")

	// 合法脚本（不调用 agent，无需 LLM）→ 成功包络
	args, _ := json.Marshal(map[string]any{
		"meta":   map[string]any{"name": "tally", "description": "sum two args"},
		"script": `return {sum: args.a + args.b};`,
		"args":   map[string]any{"a": 2, "b": 3},
	})
	out, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `workflow "tally" completed (0 agent(s)).`) ||
		!strings.Contains(out, `"sum": 5`) {
		t.Fatalf("output = %q", out)
	}

	// 非法 meta（name 非 kebab-case）→ 错误
	badMeta, _ := json.Marshal(map[string]any{
		"meta":   map[string]any{"name": "Bad Name", "description": "x"},
		"script": `return 1;`,
	})
	if _, err := tool.Execute(context.Background(), badMeta); err == nil {
		t.Fatal("invalid meta should fail")
	}

	// 非法参数 → 错误
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"meta": 1}`)); err == nil {
		t.Fatal("invalid args should fail")
	}

	// 脚本语法错误 → SCRIPT_PARSE
	syntax, _ := json.Marshal(map[string]any{
		"meta":   map[string]any{"name": "bad", "description": "x"},
		"script": `return (`,
	})
	if _, err := tool.Execute(context.Background(), syntax); err == nil ||
		!strings.Contains(err.Error(), "SCRIPT_PARSE") {
		t.Fatalf("syntax error should be SCRIPT_PARSE, got %v", err)
	}
}
