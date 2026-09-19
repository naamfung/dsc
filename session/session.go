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
        // LLMAttempt 一次 LLM 调用的结算留痕（log-only，对齐 DSH llm/* 与
        // assistant/attempt 的诊断定位：成败皆录，含 finish_reason/usage/时长/错误）。
        // 每 step 至多一条（成功）或多条（失败重试前每次尝试各一条）。
        // 该事件是排障一手证据：截断（finish_reason=max_tokens）、provider 报错、
        // 上下文溢出、流中断、耗时与 token 消耗均在此一眼可查，无需审计代码。
        LLMAttempt EventType = "llm/attempt"
        // SystemMessage 当前生效的 system prompt（surface，对齐 DSH v3 system/message）。
        // 始终占据 surface node 0：会话首条 system/message 以 append 入位，
        // 后续 prompt 变化以 replace 覆盖 node 0。空 content 表示"无 system prompt"
        // 占位节点——保留 surface 位置但不派生模型消息（投影为 nil），
        // 让"先空后非空"的 prompt 仍能 replace node 0 而非追加到 user 历史之后。
        // agent-react-loop 每轮 buildSystemPrompt 后比较 surface 当前 system 节点
        // 内容，差异即 emit 一条 SystemMessage（append 或 replace）。
        SystemMessage EventType = "system/message"
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
        SystemMessage:     true,
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
// Images 为该消息附带的图像内容寻址引用（dsc-img://<sha256>，字节持久附件库），
// 随事件日志持久化，上下文窗口内后续轮次派生历史仍会携带（对齐 rex：图片随消息保留）。
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
        // Images 工具结果图像引用（dsc-shot://<sha256> 操作截图 / dsc-img://<sha256>
        // 持久附件）。入库口（core.admitToolImages）把插件回传的 data URL 折算为引用后
        // 才进入本字段——事件日志只存引用不存字节；投影为模型历史 tool 消息的图像
        //（proto.Message.Images；视觉模型可见，dsc-shot 引用过期后降级为占位文本）。
        Images []string
}

// LLMAttemptData llm/attempt 事件载荷（log-only）：一次 LLM 调用的结算留痕。
// 成功路径：FinishReason 非空、Error 为空；失败路径：Error 非空（Code 为稳定
// 错误码），FinishReason/Usage 可能为零值（请求未达服务端或流中途断）。流式
// 中途失败时 ContentChars 记录已收到的部分内容长度（完整部分流见 assistant/chunk）。
// Usage 仅流式路径可得（finish 分片携带）；非流式协议无用量回传，保持 nil。
type LLMAttemptData struct {
        Turn, Step   int          // 所属回合/步号
        Streaming    bool         // 是否流式调用
        FinishReason string       // 结束原因（stop/tool_calls/length/max_tokens…）
        Usage        *proto.Usage // token 用量（流式 finish 分片；未知为 nil）
        DurationMS   int64        // 本次调用墙钟时长（毫秒）
        Error        string       // 失败原因文本（成功为空）
        Code         string       // 稳定错误码（context_window_exceeded/rate_limited/network_error/unknown；成功为空）
        ContentChars int          // 结算时累计内容字符数（截断/中断诊断）
        ToolCalls    int          // 结算时累计工具调用数
}
type CompactionSummaryData struct{ Content string }

// SystemMessageData system/message 事件载荷（surface，对齐 DSH v3 system/message）。
// Content 为当前生效的 system prompt 全文；空串表示"无 system prompt"占位节点
// （保留 surface node 0 位置但投影为 nil，让后续非空 prompt 仍 replace node 0）。
// Turn/Step 标记该 prompt 版本是在哪个回合的哪一步发布的，便于审计与回放定位。
type SystemMessageData struct {
        Turn, Step int
        Content    string
}

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
//
// Head 不变量（对齐 DSH v3 system/message surface node 0）：
// surface node 0 始终是 system/message（可能空 content 占位）。replace 覆盖
// node 0 时必须由 system/message 触发，且范围恰好覆盖 node 0——拒绝其他
// 事件类型重写 system head（避免 compaction 把 prompt 摘要进自己的范围）。
// 旧 v2 会话（无 system/message）经 migrateV2ToV3 在首部追加空 system/message
// 占位后也满足此不变量；非 v3 路径创建的旧会话违反不变量时，applySurfaceLocked
// 仅在 replace node 0 路径强校验，append 与 replace 其他位置照常工作。
func (s *Session) applySurfaceLocked(ev *Event) {
        switch ev.Surface.Op {
        case SurfaceAppend:
                s.surfaceNodes = append(s.surfaceNodes, ev.Seq)
        case SurfaceReplace:
                start, end := ev.Surface.Start, ev.Surface.End
                if start > end {
                        panic(fmt.Sprintf("session: replace range [%d, %d] is inverted", start, end))
                }
                // Head 不变量：覆盖 surface node 0 仅允许 system/message 触发，且范围恰好 [seq(node0), seq(node0)]
                if len(s.surfaceNodes) > 0 && start <= s.surfaceNodes[0] && end >= s.surfaceNodes[0] {
                        headSeq := s.surfaceNodes[0]
                        headEv := s.events[headSeq]
                        if headEv.Type == SystemMessage {
                                if ev.Type != SystemMessage {
                                        panic(fmt.Sprintf("session: surface replace covering node 0 (system/message) must be triggered by system/message, got %q", ev.Type))
                                }
                                if start != headSeq || end != headSeq {
                                        panic(fmt.Sprintf("session: system/message replace over node 0 must cover exactly [%d,%d], got [%d,%d]", headSeq, headSeq, start, end))
                                }
                                // 范围恰好覆盖 node 0 → 落到默认 replace 路径处理（移除 [start,end] 内节点并原位插入新节点）
                        }
                        // 若 head 不是 system/message（旧 v2 会话首条是 user/message），不强制——
                        // compaction 等可正常覆盖，迁移后此类会话首条已被替换为空 system/message
                }
                // 移除 [start, end]（seq 区间）内的 surface 节点，在首个被移除节点
                // 的原位置插入新节点。对齐 DSH surface replace 语义：replace 是
                // 位置覆盖（旧节点影子仍在日志），不是按 seq 值排序插入——
                // seq 可能在 system replace 后非单调（new system seq > old user seq），
                // 故不能用 "seq > end" 判定插入点，改用首个被移除节点的位置。
                kept := make([]int, 0, len(s.surfaceNodes))
                inserted := false
                for _, seq := range s.surfaceNodes {
                        if seq >= start && seq <= end {
                                if !inserted {
                                        kept = append(kept, ev.Seq)
                                        inserted = true
                                }
                                continue
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
// system prompt 由 surface node 0（system/message）派生，对齐 DSH v3。
func (s *Session) DeriveMessages() []*proto.Message {
        return s.DeriveMessagesLimited(-1)
}

// DeriveMessagesLimited 派生模型消息历史，并限制历史注入条数（对齐 DSH 会话级
// 运行时设置）：injectCount < 0 不限制；== 0 不注入先前轮次（仅保留当前轮，即
// 最后一条 user 消息起，模型始终能看到本次输入）；> 0 保留最近 injectCount 条
// surface 消息、并回退到最近的 user 边界——保证首条为 user（tool_use 与 tool_result
// 配对完整，Anthropic 要求 messages 以 user 开头），且当前轮总是完整保留。
// 事件日志本身不受影响（append-only，历史始终可完整恢复）。
//
// system prompt 由 surface node 0（system/message）派生：空 content 节点投影为
// nil（保留位置但不派生消息），非空 content 派生为 system 消息前置。
// surface 完全无 system/message 节点时无 system 消息前置——这违反 v3 不变量，
// 调用方应先经 migrateV2ToV3 或主动 emit SystemMessage 让 node 0 入位。
func (s *Session) DeriveMessagesLimited(injectCount int) []*proto.Message {
        s.mu.Lock()
        defer s.mu.Unlock()
        msgs := make([]*proto.Message, 0, len(s.surfaceNodes)+1)
        // 从 surface node 0 派生 system prompt（v3 设计：surface 全权负责）
        skipNode0System := false
        if len(s.surfaceNodes) > 0 {
                if d, ok := s.events[s.surfaceNodes[0]].Data.(*SystemMessageData); ok {
                        if d.Content != "" {
                                msgs = append(msgs, &proto.Message{Role: "system", Content: d.Content})
                                skipNode0System = true
                        }
                        // 空 content 节点：不派生 system 消息，但保留位置（遍历时 deriveEventMessage
                        // 也会返回 nil，重复 nil 不追加，安全）
                }
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
                // 回退到最近的 user 边界（会话首条必为 user 或 system，故终止条件必然可达）。
                for start > 0 && deriveMessageRole(s.events[s.surfaceNodes[start]]) != "user" {
                        start--
                }
                for i := start; i < len(s.surfaceNodes); i++ {
                        if skipNode0System && i == 0 {
                                continue // 已前置为 msgs[0]，不重复派生
                        }
                        if m := deriveEventMessage(s.events[s.surfaceNodes[i]]); m != nil {
                                msgs = append(msgs, m)
                        }
                }
                return msgs
        }
        for i, seq := range s.surfaceNodes {
                if skipNode0System && i == 0 {
                        continue // 已前置为 msgs[0]，不重复派生
                }
                if m := deriveEventMessage(s.events[seq]); m != nil {
                        msgs = append(msgs, m)
                }
        }
        return msgs
}

// ProjectSystemPrompt 决策当前渲染出的 system prompt 应以何种 surface 操作提交。
// 对齐 DSH SystemPromptProjection.project()：比较 rendered 与 surface 当前 system 节点
// 内容，返回应执行的 surface op：
//   - surface 无 system 节点（len==0 或 node 0 非 system/message）→ append，
//     新节点将成为 surface node 0
//   - surface 有 system 节点且 content 与 rendered 相同 → nil（no-op，无需提交）
//   - surface 有 system 节点但 content 不同：
//     inHistory=false（默认）→ replace node 0（覆盖旧 prompt）
//     inHistory=true → append 新 system/message 到 surface 尾部
//     （对齐 DSH in-history：支持读取后续 system 消息作为有效 prompt 的模型
//     可保留旧 prompt 在缓存历史中，新 prompt 追加到尾部，避免重写 node 0
//     导致的前缀缓存失效）
//
// 调用方据返回值构造 SystemMessage 事件并 Append；空 content 在首现时也提交
// （作为占位 node 0），后续非空 prompt 才能 replace 而非追加到 user 历史之后。
// 不持有 s.mu——返回纯数据，由调用方在持锁路径中 Append。
func (s *Session) ProjectSystemPrompt(rendered string, inHistory bool) (op *SurfaceOp, targetSeq int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.surfaceNodes) == 0 {
		// surface 完全空——首条 system/message 以 append 入位
		return &SurfaceOp{Op: SurfaceAppend}, -1
	}
	headSeq := s.surfaceNodes[0]
	headEv := s.events[headSeq]
	if headEv.Type != SystemMessage {
		// node 0 非 system/message（旧会话首条是 user/message）——append 一条新的 system/message
		return &SurfaceOp{Op: SurfaceAppend}, -1
	}
	// 检查 head 节点内容是否与 rendered 相同
	headUnchanged := false
	if d, ok := headEv.Data.(*SystemMessageData); ok && d.Content == rendered {
		headUnchanged = true
	}
	// in-history 路径：查找最后一个非空 system 节点（可能是 head 或更后面的 mid-history 节点）
	if inHistory {
		// 从末尾向前找最后一个非空 system/message 节点
		for i := len(s.surfaceNodes) - 1; i >= 0; i-- {
			seq := s.surfaceNodes[i]
			ev := s.events[seq]
			if ev.Type != SystemMessage {
				continue
			}
			if d, ok := ev.Data.(*SystemMessageData); ok {
				if d.Content == rendered {
					// 内容未变，无需提交
					return nil, seq
				}
				if d.Content != "" {
					// 找到最后一个非空 system 节点，内容不同 → append 新节点到尾部
					// （对齐 DSH in-history：保留旧 prompt 在缓存历史中）
					return &SurfaceOp{Op: SurfaceAppend}, seq
				}
			}
		}
		// 无非空 system 节点 → append（首个非空 prompt 入位）
		return &SurfaceOp{Op: SurfaceAppend}, -1
	}
	// 非 in-history 路径（默认）：replace node 0
	if headUnchanged {
		return nil, headSeq
	}
	// 内容变化——replace node 0（范围恰好 [headSeq, headSeq]）
	return &SurfaceOp{Op: SurfaceReplace, Start: headSeq, End: headSeq}, headSeq
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
        case *SystemMessageData:
                return "system"
        }
        return ""
}

// deriveEventMessage 单事件投影：surface 事件 → 消息；log-only 事件 → nil。
// system/message 空 content 投影为 nil（保留 surface 位置但不发模型消息，
// 对齐 DSH "空内容 system 节点投影为 null" 设计）。
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
                return &proto.Message{Role: "tool", Content: toonizeToolContent(d.Content), ToolCallId: d.CallID, Images: d.Images}
        case *CompactionSummaryData:
                return &proto.Message{Role: "user", Content: d.Content}
        case *SystemMessageData:
                // 空 content 投影为 nil（保留 surface 位置但不派生模型消息）
                if d.Content == "" {
                        return nil
                }
                return &proto.Message{Role: "system", Content: d.Content}
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
