// Package acp 是 acp-kernel 的 Go 原生重实现（对齐 MIT 协议的 acp-kernel TypeScript 核心算法）。
//
// 设计原则（对齐 acp-kernel DESIGN.md）：
//   - 纯函数式核心：零 I/O、零状态存储，state 显式传入传出（像 zip(data)→data）
//   - 模型写摘要，库编排其他一切：模型决定 *何时* 压、*压什么范围*，
//     库负责状态追踪、引用映射、prune、emergency truncate
//   - 3 层 LSM-tree：T1=直接摘要、T2=多个 T1 蒸馏、T3=多个 T2 凝结
//     （MVP 仅实现 T1，T2/T3 留接口）
//
// 与 TypeScript 版的关键差异：
//   - Go 而非 TypeScript（无 tiktoken 依赖，用字节/CJK 启发式 token 估算）
//   - 不引入 absorb / filter / wire codec / transform-channel（高级特性留后续）
//   - state 持久化由 adapter 层负责，核心不感知存储
package acp

// MessageRole 消息角色（对齐 OpenAI/Anthropic 通用语义）。
type MessageRole string

const (
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleSystem    MessageRole = "system"
	RoleTool      MessageRole = "tool"
)

// MessageContentType 消息内容类型。
type MessageContentType string

const (
	ContentTypeText       MessageContentType = "text"
	ContentTypeToolCall   MessageContentType = "tool-call"
	ContentTypeToolResult MessageContentType = "tool-result"
	ContentTypeReasoning  MessageContentType = "reasoning"
)

// CoreMessage acp 核心消息格式（对齐 acp-kernel CoreMessage）。
// adapter 层负责把 DSC proto.Message 翻译为此格式。
type CoreMessage struct {
	ID          string             `json:"id"`
	Role        MessageRole        `json:"role"`
	ContentType MessageContentType `json:"contentType"`
	Text        string             `json:"text,omitempty"`
	ToolName    string             `json:"toolName,omitempty"`
	ToolCallID  string             `json:"toolCallId,omitempty"`
}

// CompressionTier 压缩块层级。
type CompressionTier int

const (
	Tier1 CompressionTier = 1 // 直接摘要
	Tier2 CompressionTier = 2 // 多个 T1 蒸馏（MVP 留接口）
	Tier3 CompressionTier = 3 // 多个 T2 凝结（MVP 留接口）
)

// BlockGeneration 块代际（young→old 升级驱动 merge）。
type BlockGeneration string

const (
	GenYoung BlockGeneration = "young"
	GenOld   BlockGeneration = "old"
)

// CompressionBlock 一个压缩块（对齐 acp-kernel CompressionBlock）。
type CompressionBlock struct {
	BlockID            string             `json:"blockId"`
	RunID             string             `json:"runId"`
	Tier              CompressionTier    `json:"tier"`
	Topic             string             `json:"topic,omitempty"`
	Summary           string             `json:"summary"`
	DirectMessageIDs  []string           `json:"directMessageIds"`
	EffectiveMessageIDs []string         `json:"effectiveMessageIds"`
	DirectBlockIDs     []string          `json:"directBlockIds"`
	CompressedTokens   int               `json:"compressedTokens"`
	CreatedAt          int64             `json:"createdAt"`
	SurvivedCount      int               `json:"survivedCount"`
	Generation         BlockGeneration   `json:"generation"`
	Active             bool              `json:"active"`
	StartRef           string            `json:"startRef,omitempty"`
	EndRef             string            `json:"endRef,omitempty"`
}

// MessageRefMap message-id ↔ mNNNNN ref 双向映射。
type MessageRefMap struct {
	ByRaw map[string]string `json:"byRaw"` // raw id → mNNNNN
	ByRef map[string]string `json:"byRef"` // mNNNNN → raw id
}

// NudgeState nudge 决策状态（防反馈循环）。
type NudgeState struct {
	LastPerMessageNudgeTokens int            `json:"lastPerMessageNudgeTokens"`
	LastNudgeShownTokens      int            `json:"lastNudgeShownTokens"`
	BaselineTokens            int            `json:"baselineTokens"`
	Anchors                   map[string]any `json:"anchors,omitempty"`
}

// CompressionStats 压缩统计。
type CompressionStats struct {
	TokensCompressed  int `json:"tokensCompressed"`
	CompressionCount  int `json:"compressionCount"`
}

// CompressionState acp 核心状态（显式传入传出，对齐 zip(data)→data 模型）。
// adapter 层负责持久化（JSON 文件 / session 事件）。
type CompressionState struct {
	Blocks       []CompressionBlock `json:"blocks"`
	MessageRefs  MessageRefMap      `json:"messageRefs"`
	Nudge        NudgeState         `json:"nudge"`
	Stats        CompressionStats   `json:"stats"`
	NextBlockID  int                `json:"nextBlockId"`
	NextRunID    int                `json:"nextRunId"`
}

// Config 压缩配置（对齐 acp-kernel Config，精简版）。
type Config struct {
	ModelContextLimit int      `json:"modelContextLimit"` // 模型上下文窗口（token 数）
	NudgeThresholdPct  float64  `json:"nudgeThresholdPct"` // 触发 nudge 的使用率阈值（如 0.45）
	PreserveRecent     int      `json:"preserveRecent"`   // 保留尾部消息数（不被压缩）
	ProtectedTools     []string `json:"protectedTools"`   // 受保护工具名（其 tool-call/result 不被压缩）
	PromotionThreshold int      `json:"promotionThreshold"` // young→old 升级所需存活次数
}

// DefaultConfig 默认配置（对齐 acp-kernel defaultConfig）。
func DefaultConfig(modelContextLimit int) Config {
	return Config{
		ModelContextLimit: modelContextLimit,
		NudgeThresholdPct: 0.45,
		PreserveRecent:    4,
		ProtectedTools:    []string{},
		PromotionThreshold: 3,
	}
}

// CreateInitialState 创建初始空状态。
func CreateInitialState() *CompressionState {
	return &CompressionState{
		Blocks:      []CompressionBlock{},
		MessageRefs:  MessageRefMap{ByRaw: map[string]string{}, ByRef: map[string]string{}},
		Nudge:        NudgeState{Anchors: map[string]any{}},
		Stats:        CompressionStats{},
		NextBlockID:  0,
		NextRunID:    0,
	}
}

// AllocateBlockID 分配下一个 block ID（b0, b1, ...）。
func (s *CompressionState) AllocateBlockID() string {
	id := fmtBlockID(s.NextBlockID)
	s.NextBlockID++
	return id
}

// AllocateRunID 分配下一个 run ID（一次 compress 调用可能产生多个块，共享 run ID）。
func (s *CompressionState) AllocateRunID() string {
	id := fmtRunID(s.NextRunID)
	s.NextRunID++
	return id
}

// ActiveBlocks 返回所有 active 块。
func (s *CompressionState) ActiveBlocks() []CompressionBlock {
	out := make([]CompressionBlock, 0, len(s.Blocks))
	for _, b := range s.Blocks {
		if b.Active {
			out = append(out, b)
		}
	}
	return out
}

// BlockByID 按 ID 查找块（含 inactive）。
func (s *CompressionState) BlockByID(blockID string) *CompressionBlock {
	for i := range s.Blocks {
		if s.Blocks[i].BlockID == blockID {
			return &s.Blocks[i]
		}
	}
	return nil
}

// CoveredMessageIDs 返回所有 active 块覆盖的消息 ID 集合。
// 被覆盖的消息在 prune 时会被替换为 summary。
func (s *CompressionState) CoveredMessageIDs() map[string]bool {
	out := map[string]bool{}
	for _, b := range s.Blocks {
		if !b.Active {
			continue
		}
		for _, id := range b.EffectiveMessageIDs {
			out[id] = true
		}
	}
	return out
}

// HighestActiveTier 返回当前最高 active tier（用于 nudge 决策）。
func (s *CompressionState) HighestActiveTier() CompressionTier {
	var max CompressionTier
	for _, b := range s.Blocks {
		if b.Active && b.Tier > max {
			max = b.Tier
		}
	}
	return max
}

// AdvanceSurvival 推进所有 active 块的存活计数，达到阈值则升级为 old。
// 对齐 acp-kernel syncBlocks 的 advanceSurvival。
func (s *CompressionState) AdvanceSurvival(threshold int) {
	for i := range s.Blocks {
		if !s.Blocks[i].Active {
			continue
		}
		s.Blocks[i].SurvivedCount++
		if s.Blocks[i].Generation == GenYoung && s.Blocks[i].SurvivedCount >= threshold {
			s.Blocks[i].Generation = GenOld
		}
	}
}

// fmtBlockID 把数字转为 b0/b1/... 形式。
func fmtBlockID(n int) string {
	return "b" + itoa(n)
}

// fmtRunID 把数字转为 r0/r1/... 形式。
func fmtRunID(n int) string {
	return "r" + itoa(n)
}

// fmtRef 把数字转为 m00000 形式（5 位补零，对齐 acp-kernel）。
func fmtRef(n int) string {
	s := itoa(n)
	for len(s) < 5 {
		s = "0" + s
	}
	return "m" + s
}

// itoa 轻量 int → string（避免引入 strconv）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
