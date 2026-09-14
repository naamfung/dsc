package main

import (
	"strings"
)

// isTruncatedFinishReason 判定 finish_reason 是否为「输出长度截断」。
// 各 LLM 插件把上游词汇原样透传：Anthropic 系为 max_tokens，OpenAI 系为 length，
// Gemini 系为大写 MAX_TOKENS——故比较统一小写化并容忍首尾空白。
// LLM 插件已从源头移除默认 max_tokens 上限（不携带该字段、等模型自然结束），
// 本判定仅用于残余截断的显式告警与残参工具调用防护；自动续行行为待「中断」
// 根因经真机观测彻底确认后再引入。
func isTruncatedFinishReason(finishReason string) bool {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "max_tokens", "length":
		return true
	}
	return false
}
