package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestToolDisplayName 工具名统一转换为 PascalCase：read_skill → ReadSkill、
// update_goal → UpdateGoal、shell → Shell，风格不再依赖手维护映射表。
func TestToolDisplayName(t *testing.T) {
	cases := map[string]string{
		"read_skill":         "ReadSkill",
		"update_goal":        "UpdateGoal",
		"create_goal":        "CreateGoal",
		"get_goal":           "GetGoal",
		"todo_write":         "TodoWrite",
		"exit_plan_mode":     "ExitPlanMode",
		"str_replace_editor": "StrReplaceEditor",
		"lisp_eval":          "LispEval",
		"web_fetch":          "WebFetch",
		"web_search":         "WebSearch",
		"browser_screenshot": "BrowserScreenshot",
		"shell":              "Shell",
		"notify":             "Notify",
		"":                   "",
	}
	for in, want := range cases {
		if got := toolDisplayName(in); got != want {
			t.Errorf("toolDisplayName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRenderJSONCard 通用 JSON 对象卡片渲染：去掉引号/花括号等原始数据痕迹，
// 输出稳定的「key: value」键值卡片，且同层标量值列对齐。
func TestRenderJSONCard(t *testing.T) {
	raw := `{"activation":"disarmed","goal":{"id":"goal","maxGoalRounds":12,"objective":"审计目标","phase":"complete","revision":2,"roundsStarted":5}}`
	got := renderJSONCard([]byte(raw))
	want := "activation: disarmed\ngoal:\n  id           : goal\n  maxGoalRounds: 12\n  objective    : 审计目标\n  phase        : complete\n  revision     : 2\n  roundsStarted: 5"
	if got != want {
		t.Fatalf("renderJSONCard 输出不一致:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// 非对象/非法 JSON 返回空串（调用方回退原文）
	if s := renderJSONCard([]byte(`[1,2,3]`)); s != "" {
		t.Fatalf("数组不应走对象卡片: %q", s)
	}
	if s := renderJSONCard([]byte(`not json`)); s != "" {
		t.Fatalf("非法 JSON 应返回空串: %q", s)
	}
}

// TestRenderJSONCardArray 数组卡片：全标量内联、对象数组逐项展开。
func TestRenderJSONCardArray(t *testing.T) {
	raw := `{"answers":[{"id":"a","selected":["x"]},{"id":"b","selected":[]}]}`
	got := renderJSONCard([]byte(raw))
	want := "answers:\n  - id: a\n    selected: [x]\n  - id: b\n    selected: []"
	if got != want {
		t.Fatalf("对象数组卡片输出不一致:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	got = renderJSONCard([]byte(`{"tags":["go","tui"]}`))
	if got != "tags: [go, tui]" {
		t.Fatalf("全标量数组应内联: %q", got)
	}
}

// TestRenderToolResultJSONCard 工具结果帧走卡片渲染：goal 输出不再呈现为原始 JSON。
func TestRenderToolResultJSONCard(t *testing.T) {
	out := renderToolResult(`{"activation":"disarmed","goal":{"id":"goal","phase":"complete","revision":2}}`, false)
	strip := ansi.Strip(out)
	for _, want := range []string{"activation: disarmed", "goal:", "id      : goal", "phase   : complete", "revision: 2"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("结果卡片应含 %q，实际:\n%q", want, strip)
		}
	}
	if strings.Contains(strip, `"activation"`) || strings.Contains(strip, `{`) {
		t.Fatalf("结果卡片不应残留原始 JSON 痕迹，实际:\n%q", strip)
	}
}

// TestRenderViewSpecGoalCard 插件声明的结构化视图 spec 走统一渲染器：头部
// 「Goal · phase」、字段对齐，不再出现原始 JSON 的花括号/引号。
func TestRenderViewSpecGoalCard(t *testing.T) {
	view := `{"kind":"card","title":"Goal","badge":{"text":"active","tone":"green"},"fields":[{"key":"id","value":"goal"},{"key":"rounds","value":"5/12"},{"key":"revision","value":"3"},{"key":"activation","value":"armed","tone":"green"},{"key":"objective","value":"审计 llama.cpp fork"}]}`
	out := renderToolResultFrame(view, `{"goal":{}}`, false)
	strip := ansi.Strip(out)
	for _, want := range []string{"└ Goal · active", "id        : goal", "rounds    : 5/12", "revision  : 3", "activation: armed", "objective : 审计 llama.cpp fork"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("视图卡片应含 %q，实际:\n%q", want, strip)
		}
	}
	if strings.Contains(strip, `"phase"`) || strings.Contains(strip, `{`) {
		t.Fatalf("视图卡片不应残留原始 JSON，实际:\n%q", strip)
	}
	// 徽标 tone 与字段 tone 应产生 ANSI 着色
	if !strings.Contains(out, "\x1b[") {
		t.Fatalf("视图卡片应含 ANSI 着色: %q", out)
	}
	// 无视图 spec 时回退到通用结果渲染
	if got := renderToolResultFrame("", "some output", false); !strings.Contains(ansi.Strip(got), "some output") {
		t.Fatalf("无视图应回退通用渲染: %q", got)
	}
	// 非 card kind 回退通用渲染（table 等尚未实现）
	if got := renderToolResultFrame(`{"kind":"table","fields":[]}`, "row", false); !strings.Contains(ansi.Strip(got), "row") {
		t.Fatalf("未实现 kind 应回退通用渲染: %q", got)
	}
}

// TestRenderTableView 表格视图：列头 + 对齐行；超长单元格截断。
func TestRenderTableView(t *testing.T) {
	view := `{"kind":"table","title":"Search","columns":[{"key":"title","title":"Title"},{"key":"url"}],"rows":[{"title":"Go 官网","url":"https://go.dev"},{"title":"doc","url":"https://go.dev/doc"}]}`
	out := renderToolResultFrame(view, "", false)
	strip := ansi.Strip(out)
	for _, want := range []string{"└ Search", "Title", "Go 官网", "https://go.dev", "doc"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("表格应含 %q，实际:\n%q", want, strip)
		}
	}
	// 超长单元格截断为 …
	long := `{"kind":"table","title":"Files","columns":[{"key":"name"}],"rows":[{"name":"` + strings.Repeat("a", 60) + `"}]}`
	strip2 := ansi.Strip(renderToolResultFrame(long, "", false))
	if !strings.Contains(strip2, "…") {
		t.Fatalf("超长单元格应截断: %q", strip2)
	}
	// 空表格（无行）回退通用渲染
	if got := renderToolResultFrame(`{"kind":"table","columns":[{"key":"a"}],"rows":[]}`, "raw", false); !strings.Contains(ansi.Strip(got), "raw") {
		t.Fatalf("空表格应回退通用渲染: %q", got)
	}
}

// TestRenderPlainView 纯文本视图：标题 + 正文块。
func TestRenderPlainView(t *testing.T) {
	view := `{"kind":"plain","title":"result","body":"line1\nline2"}`
	out := renderToolResultFrame(view, "", false)
	strip := ansi.Strip(out)
	for _, want := range []string{"└ result", "line1", "line2"} {
		if !strings.Contains(strip, want) {
			t.Fatalf("纯文本视图应含 %q，实际:\n%q", want, strip)
		}
	}
}
