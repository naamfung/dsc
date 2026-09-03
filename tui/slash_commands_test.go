package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSlashCommandHelp 校验 /help：被处理且输出帮助文本（含各斜杆命令说明）。
func TestSlashCommandHelp(t *testing.T) {
	m := newRenderCacheModel(t)
	handled, _ := m.runSlashCommand("/help")
	if !handled {
		t.Fatal("/help should be handled")
	}
	joined := strings.Join(m.lines, "\n")
	for _, want := range []string{"/settings history", "/sandbox", "/jobs", "/session", "/export"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("/help output missing %q:\n%s", want, joined)
		}
	}
}

// TestSlashCompletionGroupFolding 校验斜杆命令分组折叠：一级菜单只显示 /settings、
// /cron 与 /jobs 入口，子命令折叠在分组内，进入（输入 "<入口> "）后展开。
func TestSlashCompletionGroupFolding(t *testing.T) {
	m := newRenderCacheModel(t)

	// 一级菜单：只有分组入口，子命令折叠在分组内
	m.input.SetValue("/")
	m.updateCompletion()
	if !m.completion.active || m.completion.kind != compSlash {
		t.Fatal("输入 / 应打开斜杆命令菜单")
	}
	for _, entry := range []string{"/settings", "/cron", "/jobs"} {
		foundEntry := false
		for _, it := range m.completion.items {
			if it.label == entry {
				foundEntry = true
			}
		}
		if !foundEntry {
			t.Fatalf("一级菜单应包含 %s 分组入口", entry)
		}
	}
	for _, sub := range []string{"/settings history", "/crons", "/cron add", "/jobs output", "/jobs kill"} {
		for _, it := range m.completion.items {
			if it.label == sub {
				t.Fatalf("一级菜单不应直接包含 %s（应折叠在分组内）", sub)
			}
		}
	}

	// 进入 /settings 分组 → 展示全部子命令
	m.input.SetValue("/settings ")
	m.updateCompletion()
	if !m.completion.active || len(m.completion.items) != 1 {
		t.Fatalf("/settings 子菜单 = %+v, want 1 个子命令", m.completion.items)
	}
	for _, want := range []string{"/settings history"} {
		found := false
		for _, it := range m.completion.items {
			if it.label == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("/settings 子菜单缺少 %q", want)
		}
	}

	// 进入 /cron 分组 → 展示全部子命令
	m.input.SetValue("/cron ")
	m.updateCompletion()
	if !m.completion.active || len(m.completion.items) != 5 {
		t.Fatalf("/cron 子菜单 = %+v, want 5 个子命令", m.completion.items)
	}
	for _, want := range []string{"/cron list", "/cron add", "/cron remove", "/cron on", "/cron off"} {
		found := false
		for _, it := range m.completion.items {
			if it.label == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("/cron 子菜单缺少 %q", want)
		}
	}

	// 进入 /jobs 分组 → 展示全部子命令
	m.input.SetValue("/jobs ")
	m.updateCompletion()
	if !m.completion.active || len(m.completion.items) != 3 {
		t.Fatalf("/jobs 子菜单 = %+v, want 3 个子命令", m.completion.items)
	}
	for _, want := range []string{"/jobs list", "/jobs output", "/jobs kill"} {
		found := false
		for _, it := range m.completion.items {
			if it.label == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("/jobs 子菜单缺少 %q", want)
		}
	}
}

// TestSlashCompletionGroupAccept 校验接受 /settings 分组入口后保持菜单打开并展开子命令
// （Enter/Tab 选中即下钻，而非直接执行）。
func TestSlashCompletionGroupAccept(t *testing.T) {
	m := newRenderCacheModel(t)
	m.input.SetValue("/settings")
	m.updateCompletion()
	if !m.completion.active || len(m.completion.items) != 1 || m.completion.items[0].label != "/settings" {
		t.Fatalf("输入 /settings 应只剩分组入口: %+v", m.completion.items)
	}
	m.acceptCompletion()
	if got := m.input.Value(); got != "/settings " {
		t.Fatalf("接受分组入口后输入应为 \"/settings \", got %q", got)
	}
	if !m.completion.active || len(m.completion.items) != 1 {
		t.Fatalf("接受分组入口后应展开 1 个子命令: %+v", m.completion.items)
	}
}

// TestSlashCommandClear 校验 /clear：清空聊天记录（含渲染缓存）。
func TestSlashCommandClear(t *testing.T) {
	m := newRenderCacheModel(t)
	m.appendMessage("line-a")
	m.appendMessage("line-b")
	if len(m.lines) == 0 {
		t.Fatal("precondition: lines should be non-empty")
	}
	handled, _ := m.runSlashCommand("/clear")
	if !handled {
		t.Fatal("/clear should be handled")
	}
	if len(m.lines) != 0 {
		t.Fatalf("/clear should empty lines, got %d", len(m.lines))
	}
	if len(m.lineRendered) != 0 || m.dirtyFrom != 0 {
		t.Fatal("/clear should reset render cache")
	}
}

// TestSlashCommandSkills 校验 /skills：空技能目录时提示尚未安装（不崩溃）。
func TestSlashCommandSkills(t *testing.T) {
	t.Setenv("DSC_SKILLS_DIR", t.TempDir())
	m := newRenderCacheModel(t)
	handled, _ := m.runSlashCommand("/skills")
	if !handled {
		t.Fatal("/skills should be handled")
	}
	joined := strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "技能") {
		t.Fatalf("/skills output missing skill heading:\n%s", joined)
	}
}

// TestSlashCommandModeNoManager 校验 /mode：无 manager 时报错而非崩溃。
func TestSlashCommandModeNoManager(t *testing.T) {
	m := newRenderCacheModel(t) // manager 为 nil
	for _, mode := range []string{"/mode minimal", "/mode standard", "/mode creation"} {
		before := len(m.lines)
		handled, _ := m.runSlashCommand(mode)
		if !handled {
			t.Fatalf("%s should be handled", mode)
		}
		if len(m.lines) <= before || !strings.Contains(strings.Join(m.lines, "\n"), "插件管理器不可用") {
			t.Fatalf("%s (no manager) should report unavailable", mode)
		}
	}
}

// TestSlashCommandExportNoManager 校验 /export：无 manager 时报错而非崩溃。
func TestSlashCommandExportNoManager(t *testing.T) {
	m := newRenderCacheModel(t)
	handled, _ := m.runSlashCommand("/export")
	if !handled {
		t.Fatal("/export should be handled")
	}
	if !strings.Contains(strings.Join(m.lines, "\n"), "插件管理器不可用") {
		t.Fatalf("/export (no manager) should report unavailable")
	}
}

// TestSlashCommandCronUsageAndNoManager 校验 /cron 用法与 remove/on/off 缺参数分支。
func TestSlashCommandCronUsageAndNoManager(t *testing.T) {
	m := newRenderCacheModel(t)
	// 无参数 → 用法提示
	handled, _ := m.runSlashCommand("/cron")
	if !handled || !strings.Contains(strings.Join(m.lines, "\n"), "用法: /cron") {
		t.Fatal("/cron should show usage")
	}
	// remove/on 无 id 时（rest 经 TrimSpace 后缺 " " 前缀）落入 default 用法提示
	handled, _ = m.runSlashCommand("/cron remove")
	if !handled || !strings.Contains(strings.Join(m.lines, "\n"), "用法: /cron add") {
		t.Fatal("/cron remove (empty id) should fall back to default usage")
	}
	handled, _ = m.runSlashCommand("/cron on")
	if !handled || !strings.Contains(strings.Join(m.lines, "\n"), "用法: /cron add") {
		t.Fatal("/cron on (empty id) should fall back to default usage")
	}
	// remove 非空 id + 无 manager → 不可用
	handled, _ = m.runSlashCommand("/cron remove cron-1")
	if !handled || !strings.Contains(strings.Join(m.lines, "\n"), "插件管理器不可用") {
		t.Fatal("/cron remove (no manager) should report unavailable")
	}
	// /cron list 与 /crons 同源：无 manager → 不可用
	for _, c := range []string{"/cron list", "/crons"} {
		handled, _ = m.runSlashCommand(c)
		if !handled || !strings.Contains(strings.Join(m.lines, "\n"), "插件管理器不可用") {
			t.Fatalf("%s (no manager) should report unavailable", c)
		}
	}
}

// TestSlashCommandQuitExit 校验 /quit 与 /exit：返回退出命令。
func TestSlashCommandQuitExit(t *testing.T) {
	for _, c := range []string{"/quit", "/exit"} {
		m := newRenderCacheModel(t)
		handled, cmd := m.runSlashCommand(c)
		if !handled {
			t.Fatalf("%s should be handled", c)
		}
		if cmd == nil {
			t.Fatalf("%s should return a quit cmd", c)
		}
	}
}

// TestSlashCompletionEscReturnsToParent 校验 esc 从分组子菜单回到上一层（一级命令菜单），
// 而非直接关闭整个菜单；未进入分组时 esc 仍关闭菜单。
func TestSlashCompletionEscReturnsToParent(t *testing.T) {
	m := newRenderCacheModel(t)

	// 进入 /settings 分组子菜单
	m.input.SetValue("/settings ")
	m.updateCompletion()
	if !m.completion.active || m.completion.kind != compSlash || len(m.completion.items) != 1 {
		t.Fatalf("前置：/settings 子菜单未就绪: %+v", m.completion.items)
	}

	// esc → 回到上一层：输入收缩为分组入口 /settings，菜单保持打开并展示一级命令
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if got := m.input.Value(); got != "/settings" {
		t.Fatalf("esc 后输入应为 \"/settings\", got %q", got)
	}
	if !m.completion.active || m.completion.kind != compSlash {
		t.Fatal("esc 回到上一层后菜单应保持打开（一级命令）")
	}
	foundEntry := false
	for _, it := range m.completion.items {
		if it.label == "/settings" {
			foundEntry = true
		}
		if it.descend == false && strings.Contains(it.label, " ") {
			t.Fatalf("一级命令不应出现子命令 %q", it.label)
		}
	}
	if !foundEntry {
		t.Fatalf("回到上一层后应可见 /settings 分组入口: %+v", m.completion.items)
	}

	// 再次 esc（此时未进入分组）→ 关闭整菜单
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.completion.active {
		t.Fatal("未进入分组时的 esc 应关闭菜单")
	}
}
