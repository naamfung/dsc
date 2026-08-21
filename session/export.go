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
		}
	}
	return b.String()
}
