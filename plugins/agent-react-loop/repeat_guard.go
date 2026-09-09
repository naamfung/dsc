package main

import (
	"encoding/json"
	"fmt"
	"path"
)

// 重复工具调用提醒（对齐 DSH repeat-tool-reminder）：统计以完全相同的规范化
// 参数连续调用同一工具的次数，达到配置阈值时注入逐级增强的提醒，要求模型
// 停止重复、重新分析上次结果。仅建议性，不否决/改写调用。
//
// 链键为 (tool name, 规范化参数)；排除工具对链透明（不增不减，记录类工具
// 穿插不能掩盖循环）；链状态进程本地（恢复会话从新链开始）。

// argumentsPreviewChars 详细提醒中引用的参数预览上限（对齐 DSH 500）。
const argumentsPreviewChars = 500

// repeatGuardTrack 更新重复链并返回应注入的提醒（空 = 未达阈值）。
// 排除工具（repeatExclude，支持 * 通配）不递增也不重置计数器。
func (a *ReactLoopAgent) repeatGuardTrack(name, canonical string) string {
	if a.repeatToolExcluded(name) {
		return ""
	}
	if name == a.repeatChainName && canonical == a.repeatChainCanonical {
		a.repeatChainCount++
	} else {
		a.repeatChainName, a.repeatChainCanonical, a.repeatChainCount = name, canonical, 1
	}
	if idx := a.repeatThresholdIdx(a.repeatChainCount); idx >= 0 {
		return repeatReminderText(name, a.repeatChainCount, canonical, idx > 0)
	}
	return ""
}

// repeatToolExcluded 判断工具是否被排除（精确或 * 通配匹配）。
func (a *ReactLoopAgent) repeatToolExcluded(name string) bool {
	for _, p := range a.repeatExclude {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}

// repeatThresholdIdx 返回 count 命中的阈值下标（-1 = 未命中）。
func (a *ReactLoopAgent) repeatThresholdIdx(count int) int {
	for i, t := range a.repeatThresholds {
		if t == count {
			return i
		}
	}
	return -1
}

// canonicalArgs 规范化工具参数：重排 JSON（Go 的 map 键序列化天然升序），
// 仅属性顺序不同的参数对象视为相同（对齐 DSH 深度排序后 stringify）。
func canonicalArgs(argsJSON string) string {
	var v any
	if err := json.Unmarshal([]byte(argsJSON), &v); err != nil {
		return argsJSON
	}
	b, err := json.Marshal(v)
	if err != nil {
		return argsJSON
	}
	return string(b)
}

// repeatReminderText 渲染提醒（对齐 DSH 文本）：首个阈值简短通用提醒，
// 后续阈值详细列出工具/连续次数/参数预览。
func repeatReminderText(name string, count int, canonical string, detailed bool) string {
	if !detailed {
		return "You are repeating the exact same tool call with identical arguments. " +
			"Carefully analyze the previous result before calling again: if the task is not " +
			"complete, try a different approach or different arguments instead of repeating the call."
	}
	args := canonical
	if len(args) > argumentsPreviewChars {
		args = args[:argumentsPreviewChars] + fmt.Sprintf("… (+%d more chars)", len(canonical)-argumentsPreviewChars)
	}
	return fmt.Sprintf("Repeated tool call detected:\n"+
		"- tool: %s\n- consecutive_calls: %d\n- arguments: %s\n"+
		"The repeated calls are not making progress. Do not call this tool with these exact "+
		"arguments again. Inspect the latest result and choose a different action, different "+
		"arguments, or finish the task if enough evidence has been gathered.", name, count, args)
}
