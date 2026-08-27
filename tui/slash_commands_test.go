package tui

import (
	"strings"
	"testing"
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
