package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// 会话 LLM 自动标题服务（对齐 DSH session-title 包）。
//
// DSH 有 3 种自动标题策略（session-title-llm / all-prompts-llm / first-prompt-llm），
// 都是 Service 子类，经 LLM 生成会话标题。
//
// DSC 的适配：提供一个 SessionTitler 接口 + LLM 实现默认后端。
// agent 在首轮完成后可选调用 Titler.Generate 生成会话标题。
// 标题用于 TUI 的 /sessions 列表展示与 admin API 的会话列表。

// SessionTitler 会话标题生成接口（对齐 DSH SessionTitleService）。
type SessionTitler interface {
	// Generate 为给定首条用户消息生成简短标题（≤50 字符）。
	Generate(ctx context.Context, firstUserMessage string) (string, error)
}

// LLMSessionTitler 使用 LLM 生成会话标题（对齐 DSH session-title-llm）。
type LLMSessionTitler struct {
	llm LLMProvider
	mu  sync.Mutex
}

// NewLLMSessionTitler 创建 LLM 标题生成器。
func NewLLMSessionTitler(llm LLMProvider) *LLMSessionTitler {
	return &LLMSessionTitler{llm: llm}
}

// sessionTitleSystemPrompt 对齐 DSH session-title-llm 的 systemPrompt（英文）。
// 输出标题使用用户消息的语言（Use the language of the messages）。
const sessionTitleSystemPrompt = "Create a concise title for an AI coding-assistant session from the supplied human messages. " +
	"Return only the title on one line, in plain text of natural language, with no quotes, prefix, explanation, Markdown, XML, or terminal control codes. " +
	"Use the language of the messages. Aim for about 5 words in non-CJK languages or 20 CJK characters."

// Generate 生成会话标题。
func (t *LLMSessionTitler) Generate(ctx context.Context, firstUserMessage string) (string, error) {
	if t.llm == nil {
		return fallbackTitle(firstUserMessage), nil
	}
	if strings.TrimSpace(firstUserMessage) == "" {
		return "New session", nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	messages := []Message{
		{Role: "system", Content: sessionTitleSystemPrompt},
		{Role: "user", Content: firstUserMessage},
	}

	resp, err := t.llm.Chat(ctx, messages, nil, 50)
	if err != nil {
		return fallbackTitle(firstUserMessage), nil // 降级到截断式标题
	}

	title := strings.TrimSpace(resp.Content)
	if title == "" {
		return fallbackTitle(firstUserMessage), nil
	}

	// 去除可能的引号
	title = strings.Trim(title, `"'""''`)
	title = strings.TrimRight(title, "。.")

	// 截断到 50 字符
	runes := []rune(title)
	if len(runes) > 50 {
		title = string(runes[:50])
	}

	return title, nil
}

// fallbackTitle 无 LLM 时的降级标题：取首条消息的前 20 个字符。
func fallbackTitle(firstUserMessage string) string {
	msg := strings.TrimSpace(firstUserMessage)
	if msg == "" {
		return "New session"
	}
	runes := []rune(msg)
	if len(runes) > 20 {
		return string(runes[:20]) + "…"
	}
	return msg
}

// FormatSessionTitle 格式化会话标题用于展示（对齐 DSH 的展示格式）。
func FormatSessionTitle(title string) string {
	if title == "" {
		return "未命名会话"
	}
	return fmt.Sprintf("%s", title)
}
