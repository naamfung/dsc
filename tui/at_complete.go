package tui

import (
	"os"
	"sort"
	"strings"
)

// 补全菜单尺寸上限（对齐 REX）：菜单最多同时显示 maxCompRows 行（窗口滚动），
// 单个目录最多贡献 maxCompItems 个候选，防病态大目录撑爆菜单/压垮消息区。
const (
	maxCompRows  = 8
	maxCompItems = 100
)

// activeAtToken 查找输入末尾（光标处）的 @ 引用 token。@ 必须位于行首或紧跟
// 空白/换行（避免 "a@b" 触发）；反斜杆转义的空格/tab 属于 token（补全对含空格
// 路径的插入形式）。返回 '@' 的字节偏移与其后文本。
func activeAtToken(val string) (int, string, bool) {
	for i := len(val) - 1; i >= 0; i-- {
		switch val[i] {
		case ' ', '\t':
			if i > 0 && val[i-1] == '\\' {
				i-- // 转义空白仍在 token 内
				continue
			}
			return 0, "", false
		case '\n':
			return 0, "", false
		case '@':
			if i == 0 || val[i-1] == ' ' || val[i-1] == '\t' || val[i-1] == '\n' {
				return i, val[i+1:], true
			}
			return 0, "", false
		}
	}
	return 0, "", false
}

// splitPathToken 把 @ token 按最后一个 '/' 拆成 (dir, frag)：dir 保留尾部斜杆
// （如 "sub/dir/"），frag 为正在输入的一段。
func splitPathToken(token string) (dir, frag string) {
	if i := strings.LastIndex(token, "/"); i >= 0 {
		return token[:i+1], token[i+1:]
	}
	return "", token
}

// setCompletion 安装菜单；同一菜单种类保持选中索引（避免每次按键重置到顶部）。
func (m *Model) setCompletion(kind compKind, items []compItem, replaceFrom int) {
	sel := 0
	if m.completion.active && m.completion.kind == kind && m.completion.sel < len(items) {
		sel = m.completion.sel
	}
	m.completion = completion{active: true, kind: kind, items: items, sel: sel, replaceFrom: replaceFrom}
}

// fileItems 为 @ token 列出目录一层候选：dir 为 '/' 之前部分、frag 为正在输入的
// 部分；目录优先且可下钻（选中后保持菜单打开进入下一层），文件补全。隐藏项默认
// 跳过（frag 以 '.' 开头时显示）。工作区根（DSC_WORKSPACE_ROOT）为默认根，与
// ResolveImageRefs 的 @ 引用解析保持一致。
func (m *Model) fileItems(token string) []compItem {
	dir, frag := splitPathToken(token)
	wsRoot := os.Getenv("DSC_WORKSPACE_ROOT")
	fsFrag := unescapeRefPath(frag)
	readDir := resolveRefPath(wsRoot, unescapeRefPath(dir))
	entries, err := os.ReadDir(readDir)
	if err != nil {
		entries = nil
	}
	// 目录优先（ReadDir 本身按名排序，保持稳定序）
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].IsDir() && !entries[j].IsDir()
	})
	showHidden := strings.HasPrefix(fsFrag, ".")
	var items []compItem
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, fsFrag) {
			continue
		}
		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			items = append(items, compItem{label: name + "/", insert: "@" + dir + escapeRefPath(name) + "/", hint: "目录", descend: true})
		} else {
			items = append(items, compItem{label: name, insert: "@" + dir + escapeRefPath(name)})
		}
		if len(items) >= maxCompItems {
			break
		}
	}
	return items
}
