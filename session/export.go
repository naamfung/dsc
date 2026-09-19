package session

import (
        "fmt"
        "strings"
)

// ExportTranscript 将会话事件日志投影为人类可读的 Markdown 记录（transcript）。
// 与派生消息不同，transcript 遍历**原始事件**（含被压缩 replace 遮蔽的历史），
// 因此是完整的会话记录；tool 调用与结果配对展示，压缩摘要单独标注。
func (s *Session) ExportTranscript() string {
        s.mu.Lock()
        defer s.mu.Unlock()
        var b strings.Builder
        events := s.events
        if len(events) == 0 {
                return "(空会话)\n"
        }
        b.WriteString(fmt.Sprintf("# 会话导出（%d 事件）\n\n", len(events)))
        for _, ev := range events {
                switch ev.Type {
                case TurnStart:
                        if d, ok := ev.Data.(*TurnData); ok {
                                fmt.Fprintf(&b, "\n## 轮次 %d\n\n", d.Turn)
                        }
                case UserMessage:
                        if d, ok := ev.Data.(*UserMessageData); ok {
                                src := "用户"
                                if d.Source != "" && d.Source != "user" {
                                        src = "上下文(" + d.Source + ")"
                                }
                                fmt.Fprintf(&b, "**%s**: %s\n\n", src, d.Content)
                        }
                case AssistantMessage:
                        if d, ok := ev.Data.(*AssistantMessageData); ok {
                                if d.Reasoning != "" {
                                        fmt.Fprintf(&b, "> 思考：%s\n\n", d.Reasoning)
                                }
                                if d.Content != "" {
                                        fmt.Fprintf(&b, "**助手**: %s\n\n", d.Content)
                                }
                                if len(d.ToolCalls) > 0 {
                                        var calls []string
                                        for _, tc := range d.ToolCalls {
                                                calls = append(calls, fmt.Sprintf("%s(%s)", tc.Name, tc.ArgumentsJson))
                                        }
                                        fmt.Fprintf(&b, "**工具调用**: %s\n\n", strings.Join(calls, ", "))
                                }
                        }
                case ToolResult:
                        if d, ok := ev.Data.(*ToolResultData); ok {
                                if d.Error != "" {
                                        fmt.Fprintf(&b, "  → 工具结果(%s) 错误: %s\n\n", d.CallID, d.Error)
                                } else {
                                        fmt.Fprintf(&b, "  → 工具结果(%s): %s\n\n", d.CallID, d.Content)
                                }
                        }
                case CompactionSummary:
                        if d, ok := ev.Data.(*CompactionSummaryData); ok {
                                fmt.Fprintf(&b, "**摘要（上下文压缩）**: %s\n\n", d.Content)
                        }
                case LLMAttempt:
                        // LLM 调用结算留痕（log-only）：排障一手证据，紧凑单行。
                        if d, ok := ev.Data.(*LLMAttemptData); ok {
                                kind := "非流式"
                                if d.Streaming {
                                        kind = "流式"
                                }
                                extra := ""
                                if d.Error != "" {
                                        extra = fmt.Sprintf(" 错误=%s", d.Error)
                                        if d.Code != "" && d.Code != "unknown" {
                                                extra = fmt.Sprintf(" 错误=%s(%s)", d.Error, d.Code)
                                        }
                                }
                                usage := ""
                                if d.Usage != nil {
                                        usage = fmt.Sprintf(" tokens{%d/%d/%d}",
                                                d.Usage.PromptTokens, d.Usage.CompletionTokens, d.Usage.TotalTokens)
                                }
                                fmt.Fprintf(&b, "  · LLM %s调用: finish=%s 时长=%dms 内容=%d字符 工具=%d%s%s\n\n",
                                        kind, orDefault(d.FinishReason, "无"), d.DurationMS,
                                        d.ContentChars, d.ToolCalls, usage, extra)
                        }
                case SystemMessage:
                        // system/message 是模型历史而非对话——人类导出跳过（对齐 DSH：
                        // "Human transcript projections skip system/message; it is model history,
                        // not conversation"）。完整事件日志已含此事件，/export 仅省略渲染。
                }
        }
        return b.String()
}

// orDefault 返回 s，为空时返回 fallback（导出渲染用的小助手）。
func orDefault(s, fallback string) string {
        if s == "" {
                return fallback
        }
        return s
}
