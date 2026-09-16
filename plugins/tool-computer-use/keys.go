package main

import (
	"fmt"
	"strings"
)

// keyAliases 常用键名别名 → robotgo 键名（robotgo 自身还带一层 Special 映射，
// 此处只收口跨平台文档中高频的别名，未知名字原样小写透传）。
var keyAliases = map[string]string{
	"control":   "ctrl",
	"return":    "enter",
	"escape":    "esc",
	"del":       "delete",
	"ins":       "insert",
	"super":     "cmd",
	"meta":      "cmd",
	"win":       "cmd",
	"windows":   "cmd",
	"option":    "alt",
	"spacebar":  "space",
	"pgup":      "pageup",
	"pgdn":      "pagedown",
	"pgdown":    "pagedown",
	"backspace": "backspace",
	"plus":      "=",
}

// validModifiers 合法修饰键集合（对齐 robotgo 约定与工具 schema 描述）。
var validModifiers = map[string]bool{
	"ctrl":  true,
	"alt":   true,
	"shift": true,
	"cmd":   true,
}

// normalizeKey 归一化键名：trim + 小写 + 别名映射；空串报错。
func normalizeKey(name string) (string, error) {
	k := strings.ToLower(strings.TrimSpace(name))
	if k == "" {
		return "", fmt.Errorf("key name is required")
	}
	if mapped, ok := keyAliases[k]; ok {
		k = mapped
	}
	return k, nil
}

// normalizeButton 归一化鼠标按键名（缺省 left）；middle 统一为 robotgo 的
// middle 按钮；未知值报错而非静默左键（避免模型手滑产生误操作）。
func normalizeButton(name string) (string, error) {
	b := strings.ToLower(strings.TrimSpace(name))
	if b == "" {
		return "left", nil
	}
	switch b {
	case "left", "right", "middle", "center":
		if b == "center" {
			return "middle", nil
		}
		return b, nil
	default:
		return "", fmt.Errorf("button must be one of left/right/middle, got %q", name)
	}
}
