// Package session 提供事件溯源的会话日志：agent 交互历史的唯一事实源。
//
// 对齐 DSH core/session 的设计：仅追加的事件日志 + surface 投影 + 派生模型历史。
// 模型消息历史由 DeriveMessages 从 surface 派生，从不单独存储。
// 当前为内存实现；JSONL 持久化与回放恢复见 persist.go。
package session

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/toon-format/toon-go"

	"dsc/proto"
)

// EventType 会话事件类型。词汇表可扩展（未来可新增类型，如持久化/回放扩展）。
type EventType string

const (
	// TurnStart/TurnEnd 轮次边界（一次模型循环执行）。
	TurnStart EventType = "turn/start"
	TurnEnd   EventType = "turn/end"
	// StepStart/StepEnd 步骤边界（一次模型调用 + 其请求的工具执行）。
	StepStart EventType = "step/start"
	StepEnd   EventType = "step/end"
	// UserMessage 用户侧消息（surface）。
	UserMessage EventType = "user/message"
	// AssistantChunk 原始流分片（log-only：回放/UI 保真，不参与派生历史）。
	AssistantChunk EventType = "assistant/chunk"
	// AssistantMessage 组装后的助手消息（surface）。
	AssistantMessage EventType = "assistant/message"
	// ToolCallEvent 模型请求的一次工具调用（log-only，与 ToolResult 配对）。
	ToolCallEvent EventType = "tool/call"
	// ToolResult 工具执行结果（surface）。
	ToolResult EventType = "tool/result"
	// CompactionSummary 上下文压缩的摘要节点（surface replace 遮蔽被压缩范围）。
	CompactionSummary EventType = "compaction/summary"
	// PlanMode plan 模式状态（log-only：整值替换，最后一条生效，fold 恢复）。
	PlanMode EventType = "plan/mode"
	// HistoryLimit 历史注入条数上限（log-only：整值替换，最后一条生效，fold 恢复）。
	// 与 plan/mode 同类：属于会话级运行时设置，随事件日志持久化，重启/切换后折叠还原。
	HistoryLimit EventType = "history/limit"
	// GoalChange 目标状态变更（log-only：携带完整快照；clear 为 tombstone）。
	GoalChange EventType = "goal/change"
	// TodoWrite 任务清单整表替换（log-only：投影/UI 状态，不进模型历史；
	// 每个 turn/start 使当前有效计划失效，见 FoldTodos）。
	TodoWrite EventType = "todo/write"
	// ApprovalAsked/ApprovalDecided 审批审计事件（log-only：沙箱升级审批的提问与结论，
	// 对齐 DSH approval/asked + approval/decided 会话审计；由 agent 经宿主 OnEvent 桥落盘）。
	ApprovalAsked   EventType = "approval/asked"
	ApprovalDecided EventType = "approval/decided"
	// ApprovalPolicy 审批策略（log-only：整值替换，最后一条生效，fold 恢复；缺省回退部署值）。
	// 对齐 DSH approval/policy 会话态：per-session，resume/fork 后经事件日志折叠还原。
	ApprovalPolicy EventType = "approval/policy"
)

// Surface op 取值。
const (
	SurfaceAppend  = "append"
	SurfaceReplace = "replace"
)

// surfaceEventTypes 产生模型消息的事件类型（参与派生历史）。
var surfaceEventTypes = map[EventType]bool{
	UserMessage:       true,
	AssistantMessage:  true,
	ToolResult:        true,
	CompactionSummary: true,
}

// SurfaceOp 声明事件如何进入有序 surface。
type SurfaceOp struct {
	// Op: SurfaceAppend 尾部追加；SurfaceReplace 遮蔽 [Start, End]（含）范围内的 surface 节点。
	Op    string
	Start int
	End   int
}

// 各事件类型的 payload（类型安全判别联合）。
type TurnData struct {
	Turn   int
	Reason string
}
type StepData struct{ Turn, Step int }

// UserMessageData 用户消息事件载荷。
// Images 为该消息附带的图像 data URL（data:image/...;base64,...），随事件日志持久化，
// 上下文窗口内后续轮次派生历史仍会携带（对齐 rex：图片随消息保留）。
type UserMessageData struct {
	Content string
	Source  string
	Images  []string
}
type AssistantChunkData struct {
	Turn, Step         int
	Content, Reasoning string
}
type AssistantMessageData struct {
	Turn, Step  int
	Content     string
	Reasoning   string
	ToolCalls   []*proto.ToolCall
	Usage       *proto.Usage
	Interrupted bool
}
type ToolCallData struct {
	Turn, Step              int
	CallID, Name, Arguments string
}
type ToolResultData struct {
	Turn, Step             int
	CallID, Content, Error string
}
type CompactionSummaryData struct{ Content string }

// HistoryLimitData history/limit 事件载荷（log-only）：历史注入条数上限。
// Count < 0 表示不限制（缺省）；0 表示不注入历史；>0 表示只注入最近 Count 条。
type HistoryLimitData struct {
	Count int
}

// Event 一条会话日志条目。Seq 单调连续（= 日志长度），Time 为 epoch 毫秒。
type Event struct {
	Seq     int
	Time    int64
	Type    EventType
	Data    any
	Surface *SurfaceOp // 仅 surface 事件携带
}

// Session 事件溯源会话：append-only 事件日志 + 有序 surface 投影。
// 派生历史由 surface 节点按事件顺序投影得出。
type Session struct {
	mu sync.Mutex
	// id 会话标识（由 Store 分配/设置；直接 New 创建的会话为空，需 Store 管理才可落盘）。
	id string
	// events 日志本体；seq 恒等于其在切片中的下标。
	events []*Event
	// surfaceNodes 当前派生历史中的 surface 事件 seq，按事件顺序。
	surfaceNodes []int
	// replaceGen 已提交的 positional replace 次数，供消费方区分尾部增长与重写。
	replaceGen int
}

// New 创建空会话。
func New() *Session { return &Session{} }

// ID 返回会话标识（由 Store 分配；直接 New 创建的会话为空）。
func (s *Session) ID() string { return s.id }

// Len 返回日志长度（下一事件的 seq）。
func (s *Session) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

// Events 返回事件日志的只读快照。
func (s *Session) Events() []*Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Event, len(s.events))
	copy(out, s.events)
	return out
}

// ReplaceGeneration 返回已提交的 surface 重写次数。
func (s *Session) ReplaceGeneration() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceGen
}

// SurfaceNodes 返回当前派生历史中的 surface 事件 seq（事件顺序）。
func (s *Session) SurfaceNodes() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int, len(s.surfaceNodes))
	copy(out, s.surfaceNodes)
	return out
}

// Append 追加一条事件。surface 事件必须携带 SurfaceOp；log-only 事件必须为 nil。
// 违反契约（非 surface 类型携带 SurfaceOp / 未知 op）在追加处 panic——
// 与 DSH 一致：错误事件在源头失败，绝不让坏事件进入日志。
func (s *Session) Append(typ EventType, data any, surface *SurfaceOp) *Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev := &Event{
		Seq:     len(s.events),
		Time:    time.Now().UnixMilli(),
		Type:    typ,
		Data:    data,
		Surface: surface,
	}
	if surface != nil {
		if !surfaceEventTypes[typ] {
			panic(fmt.Sprintf("session: non-surface event type %q cannot carry surface op", typ))
		}
		s.applySurfaceLocked(ev)
	}
	s.events = append(s.events, ev)
	return ev
}

// applySurfaceLocked 将 surface 事件并入有序 surface（需已持有 s.mu）。
func (s *Session) applySurfaceLocked(ev *Event) {
	switch ev.Surface.Op {
	case SurfaceAppend:
		s.surfaceNodes = append(s.surfaceNodes, ev.Seq)
	case SurfaceReplace:
		start, end := ev.Surface.Start, ev.Surface.End
		if start > end {
			panic(fmt.Sprintf("session: replace range [%d, %d] is inverted", start, end))
		}
		// 移除 [start, end]（事件 seq 区间）内的 surface 节点，在原位置插入新节点。
		kept := s.surfaceNodes[:0]
		inserted := false
		for _, seq := range s.surfaceNodes {
			if seq >= start && seq <= end {
				continue
			}
			if !inserted && seq > end {
				kept = append(kept, ev.Seq)
				inserted = true
			}
			kept = append(kept, seq)
		}
		if !inserted {
			kept = append(kept, ev.Seq)
		}
		s.surfaceNodes = kept
		s.replaceGen++
	default:
		panic(fmt.Sprintf("session: unknown surface op %q", ev.Surface.Op))
	}
}

// DeriveMessages 从 surface 投影派生模型消息历史（[]*proto.Message，兼容 LLM 调用）。
// sysPrompt 前置为 system 消息（来自宿主，不入事件日志）。
func (s *Session) DeriveMessages(sysPrompt string) []*proto.Message {
	return s.DeriveMessagesLimited(sysPrompt, -1)
}

// DeriveMessagesLimited 派生模型消息历史，并限制历史注入条数（对齐 DSH 会话级
// 运行时设置）：injectCount < 0 不限制；== 0 不注入先前轮次（仅保留当前轮，即
// 最后一条 user 消息起，模型始终能看到本次输入）；> 0 保留最近 injectCount 条
// surface 消息、并回退到最近的 user 边界——保证首条为 user（tool_use 与 tool_result
// 配对完整，Anthropic 要求 messages 以 user 开头），且当前轮总是完整保留。
// 事件日志本身不受影响（append-only，历史始终可完整恢复）。
func (s *Session) DeriveMessagesLimited(sysPrompt string, injectCount int) []*proto.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs := make([]*proto.Message, 0, len(s.surfaceNodes)+1)
	if sysPrompt != "" {
		msgs = append(msgs, &proto.Message{Role: "system", Content: sysPrompt})
	}
	if injectCount >= 0 {
		// 起点：最近 injectCount 条消息的开头；injectCount==0 表示只保留当前轮。
		start := len(s.surfaceNodes) - injectCount
		if injectCount == 0 {
			start = len(s.surfaceNodes) - 1
		}
		if start < 0 {
			start = 0
		}
		// 回退到最近的 user 边界（会话首条必为 user，故终止条件必然可达）。
		for start > 0 && deriveMessageRole(s.events[s.surfaceNodes[start]]) != "user" {
			start--
		}
		for i := start; i < len(s.surfaceNodes); i++ {
			if m := deriveEventMessage(s.events[s.surfaceNodes[i]]); m != nil {
				msgs = append(msgs, m)
			}
		}
		return msgs
	}
	for _, seq := range s.surfaceNodes {
		if m := deriveEventMessage(s.events[seq]); m != nil {
			msgs = append(msgs, m)
		}
	}
	return msgs
}

// deriveMessageRole 返回 surface 事件派生的消息角色（用于截断边界判定）。
func deriveMessageRole(ev *Event) string {
	switch ev.Data.(type) {
	case *UserMessageData, *CompactionSummaryData:
		return "user"
	case *AssistantMessageData:
		return "assistant"
	case *ToolResultData:
		return "tool"
	}
	return ""
}

// deriveEventMessage 单事件投影：surface 事件 → 消息；log-only 事件 → nil。
func deriveEventMessage(ev *Event) *proto.Message {
	switch d := ev.Data.(type) {
	case *UserMessageData:
		return &proto.Message{Role: "user", Content: d.Content, Images: d.Images}
	case *AssistantMessageData:
		// 内容为空且无工具调用时跳过（同 DSH：空 assistant 不得进入模型历史）。
		if d.Content == "" && len(d.ToolCalls) == 0 {
			return nil
		}
		m := &proto.Message{Role: "assistant", Content: d.Content}
		if len(d.ToolCalls) > 0 {
			m.ToolCalls = d.ToolCalls
		}
		return m
	case *ToolResultData:
		return &proto.Message{Role: "tool", Content: toonizeToolContent(d.Content), ToolCallId: d.CallID}
	case *CompactionSummaryData:
		return &proto.Message{Role: "user", Content: d.Content}
	}
	return nil
}

// toonizeToolContent 在「事件 → 模型消息」投影层把工具结果的 Content 做确定性
// TOON 规范化：当结果为结构化 JSON（对象/数组）时转成更紧凑的表式表示，以更少
// token 投喂模型、减少上下文噪音；非结构内容（纯文本、标量、null）与无法解析的
// 串一律原样保留。转换是纯函数（map 按键排序输出），同样的 JSON 恒得同样的 TOON，
// 因此不破坏前缀缓存稳定。事件日志原文与 TUI 展示不受影响（仅在投喂模型时生效）。
func toonizeToolContent(content string) string {
	var v any
	if err := json.Unmarshal([]byte(content), &v); err != nil {
		return content
	}
	switch v.(type) {
	case map[string]any, []any:
		// 仅对有序结构做紧凑化；标量 / null 无结构，转换无益。
	default:
		return content
	}
	encoded, err := toon.MarshalString(v)
	if err != nil {
		return content // 降级：TOON 失败则保留原文 JSON，绝不断裂投喂
	}
	return encoded
}
