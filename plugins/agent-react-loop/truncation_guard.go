package main

import (
	"strings"
)

// isTruncatedFinishReason 判定 finish_reason 是否为「输出长度截断」。
// 各 LLM 插件把上游词汇原样透传：Anthropic 系为 max_tokens，OpenAI 系为 length，
// Gemini 系为大写 MAX_TOKENS——故比较统一小写化并容忍首尾空白。
// LLM 插件已从源头移除默认 max_tokens 上限（不携带该字段、等模型自然结束），
// 残余截断来自 provider 侧默认输出上限（真机实测：anthropic 兼容口把 6144 词元
// 的长报告拦腰截断）。收尾侧按本判定做两件事：残参工具调用拒执行（防半截
// JSON 下发），以及预算内「从中断处继续」自动续行（截断不弃任务，见 main.go
// continuation drivers）。
func isTruncatedFinishReason(finishReason string) bool {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "max_tokens", "length":
		return true
	}
	return false
}
