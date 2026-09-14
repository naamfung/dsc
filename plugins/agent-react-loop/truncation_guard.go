package main

import (
	"fmt"
	"strings"
)

// isTruncatedFinishReason 判定 finish_reason 是否为「输出长度截断」。
// 各 LLM 插件把上游词汇原样透传：Anthropic 系为 max_tokens，OpenAI 系为 length，
// Gemini 系为大写 MAX_TOKENS——故比较统一小写化并容忍首尾空白。
// 此前全链路无人读取 finish_reason，截断响应被当普通完成收轮：纯文本被拦腰
// 切断时轮次静默结束（用户感知为「无故中断连接」），是反复出现的顽疾。
func isTruncatedFinishReason(finishReason string) bool {
	switch strings.ToLower(strings.TrimSpace(finishReason)) {
	case "max_tokens", "length":
		return true
	}
	return false
}

// buildTruncationNudgePrompt 构造截断续行追问：作为 user 消息追加进派生历史，
// 让模型从被切断处继续，而非把半句答复晾给用户（被感知为「无故中断连接」）。
// 文案显式包含截断标志词汇（max_tokens/截断），便于日志与回归测试识别续行动因。
func buildTruncationNudgePrompt(finishReason string) string {
	return fmt.Sprintf(
		"你的上一条回复因达到输出长度上限被截断（finish_reason=%s，即 max_tokens/length 类截断标志），话说了一半即停止。"+
			"请从中断处无缝继续完成你的回答或任务：不要重复已输出的内容，不要致歉或解释截断本身；"+
			"若尚有步骤未完成（含工具调用），请继续执行。", finishReason)
}
