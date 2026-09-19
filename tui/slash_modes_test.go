package tui

import (
	"strings"
	"testing"
)

// TestSlashCommandMenuIncludesModeEntries 校验斜杆命令菜单包含 /mode 项。
// 无 manager 时回退到内置默认（minimal / standard / creation）。
func TestSlashCommandMenuIncludesModeEntries(t *testing.T) {
	m := newRenderCacheModel(t) // manager = nil → 默认模式

	items := m.slashCommandItems()
	var modeItems []string
	for _, it := range items {
		if strings.HasPrefix(it.label, "/mode ") {
			modeItems = append(modeItems, it.label)
		}
	}
	if len(modeItems) == 0 {
		t.Fatal("slashCommandItems 应包含 /mode 项")
	}
	for _, want := range []string{"/mode minimal", "/mode standard", "/mode creation"} {
		found := false
		for _, got := range modeItems {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("默认模式列表缺少 %s，现有：%v", want, modeItems)
		}
	}
}

// TestSlashCompletionShowsModeEntries 校验输入 / 时菜单实际呈现 /mode 项。
func TestSlashCompletionShowsModeEntries(t *testing.T) {
	m := newRenderCacheModel(t)
	m.input.SetValue("/")
	m.updateCompletion()
	if !m.completion.active {
		t.Fatal("输入 / 应打开斜杆命令菜单")
	}
	foundMinimal := false
	for _, it := range m.completion.items {
		if it.label == "/mode minimal" {
			foundMinimal = true
			break
		}
	}
	if !foundMinimal {
		t.Fatalf("/ 菜单应包含 /mode minimal，现有：%v", m.completion.items)
	}
}

// TestSlashCommandHelpIncludesModeEntries 校验 /help 输出含 /mode 行。
func TestSlashCommandHelpIncludesModeEntries(t *testing.T) {
	m := newRenderCacheModel(t)
	handled, _ := m.runSlashCommand("/help")
	if !handled {
		t.Fatal("/help 应被处理")
	}
	joined := strings.Join(m.lines, "\n")
	for _, want := range []string{"/mode minimal", "/mode standard", "/mode creation"} {
		if !strings.Contains(joined, want) {
			t.Errorf("/help 输出缺少 %q", want)
		}
	}
}

// TestSlashCommandUnknownModeIsHandled 校验 /mode <未知名> 走 default 分支
// 而非被当作普通消息发送（避免用户新增 preset 后命令"消失"）。
func TestSlashCommandUnknownModeIsHandled(t *testing.T) {
	m := newRenderCacheModel(t) // manager = nil → 会报"插件管理器不可用"
	handled, _ := m.runSlashCommand("/mode thin")
	if !handled {
		t.Fatal("/mode thin 应被识别为斜杆命令并处理（即使 manager 不可用）")
	}
	joined := strings.Join(m.lines, "\n")
	// manager 不可用时应报错，而非把 /mode thin 当普通消息发出去
	if !strings.Contains(joined, "插件管理器不可用") {
		t.Fatalf("/mode thin (no manager) 应报错，现有：%s", joined)
	}
}

// TestSlashCommandModeUsageError 校验 /mode（无参数）与 /mode foo bar（多 token）
// 给出用法提示而非走 default 分支尝试切换。
func TestSlashCommandModeUsageError(t *testing.T) {
	m := newRenderCacheModel(t)
	for _, cmd := range []string{"/mode ", "/mode foo bar"} {
		m.lines = nil
		handled, _ := m.runSlashCommand(cmd)
		if !handled {
			t.Fatalf("%q 应被处理", cmd)
		}
		joined := strings.Join(m.lines, "\n")
		if !strings.Contains(joined, "用法") {
			t.Errorf("%q 应给出用法提示，现有：%s", cmd, joined)
		}
	}
}
