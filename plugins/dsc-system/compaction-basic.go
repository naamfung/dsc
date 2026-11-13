// dsc-system 驻留：基础上下文压缩（对齐 DSH compaction-basic）。
// 自宿主 core/compaction.go 迁入——压缩策略与状态归还插件，宿主零压缩代码；
// 经通用（dsc）类型的 agent/pre-step 事件接管机制生效（与 dsc-billion-context
// 同一协议，agent 与协议零改动）：
//
//   - agent/pre-step（waterfall 拦截）：收到本步消息列表（MessagesJSON）；
//     估算用量超过阈值（默认窗口 70%，对齐 compaction-basic 压力语义；接管靠
//     能力声明驱动——谁声明 Provides compaction 且当值，谁收到完整消息列表，
//     与阈值无关）时保留尾部（RetainRatio / RetainTokensMin 取大），把未压缩前段
//     **按块**经 LLM 生成摘要（interconnect 聚合 LLM；未互联或调用失败退化为截断式
//     摘要），以 {"messages": [...]} 返回改写后的消息列表。
//
// 为什么阈值是 70% 而不是 80%：宿主的收口预算（enforceRequestCeiling）是窗口的 80%
// ——它是「估算可能偏低」的兜底余量。压缩阈值与收口预算贴得越近，留给偏差的空间越小：
// 实测一次真实事故里，压力判到 80% 才触发，压缩还没完成宿主就撞上收口，整轮失败。
// 留 10 个点给估算误差与一轮的增量（工具结果常常一次几万 token），压得早一点、稳一点。
//
// 为什么摘要要分块 + 限时：压缩是在 agent/pre-step 里**同步**跑的，而宿主的整条事件
// 分发只有 30s 预算（DSC_EVENT_DISPATCH_TIMEOUT 可放大）。一次把 20 万 token 塞进一条
// 摘要 prompt，既跑不完（超时后宿主的改写被丢弃，全量历史照旧发出去），又会退化成截断式
// 摘要（15 万 token 换一句话，永久落盘）。分块后每块都能在预算内跑完；跑不完就停在已完成
// 的块上（前缀已经缩短，下一轮从这里继续），绝不为了「一次压完」丢整段历史。
//
// 为什么块大小必须按**实测速度**算，而不是定死 50K：预算（20s）与分发超时（30s）是时间
// 预算，摘要耗时却由模型速度决定。云端模型 prefill 几万 tok/s，50K 的块两三秒读完；
// 本地引擎只有 1~2K tok/s、decode 20~40 tok/s——50K 的块光读 prompt 就要 30s+，再加
// 2K 输出，**每一轮都必然撞满预算**。现场就是这样：同一个 40,331 token 的块被重发 144
// 次（每次都在 ~21s 处被掐断、读进度停在 32,768），一次也没压成，而宿主侧一条压缩记录
// 都没有，上下文一路涨到窗口的 88%，用户看到的是「过了阈值却始终没压缩」。
// 所以：块大小与输出上限都由「剩余预算 × 吞吐估计」推出，吞吐估计每轮按实测修正
// （撞预算就减半，跑得快就放大），任何速度的引擎都能在预算内跑完第一块。
//
// 溢出（provider 4xx）不由本插件处理：超窗的处置权归宿主（滑窗，无损），本插件只在
// pre-step 按压力改写。曾经的「request-error 紧急压缩」会跳过 LLM、用截断摘要把整段
// 真实消息永久压掉，已随那次裁定删除。
//
// per-session 状态：pre-step 改写只作用于**本次** LLM 请求（agent 内部历史不改写，
// 会话日志一字不改）。逐 step 复用靠 per-session 记录：记已压缩前缀的指纹（sha256）+
// 摘要 + 审计字段；指纹吻合直接复用摘要、只对新溢出段追加压缩，失配（历史被改写/
// 回滚）则整套作废从头评估。
//
// 这张记录会落到插件自己的 <session>.dsc-system.json（见 compaction-state.go）：
// 进程重启后指纹仍吻合即复用，不必为同一段前缀重新调用 LLM。它**不是**会话日志，
// 也不参与历史派生——丢掉它只会让下次重压一次，绝不改变模型看到的历史（与 DSH 把
// 压缩写成会话 surface 事件、遮蔽历史、不可逆的做法有本质区别）。
//
// 配置（DSC_ 前缀 env 白名单天然可见）：
//   - DSC_COMPACTION_BASIC_CONTEXT_WINDOW  上下文窗口 token 数（默认 131072；0 = 驻留禁用）
//   - DSC_COMPACTION_BASIC_THRESHOLD       触发阈值比例（默认 0.70，须低于宿主收口预算
//     0.80 才留得出余量；可调，与接管机制无关）
//   - DSC_COMPACTION_BASIC_RETAIN_RATIO    保留尾部比例（默认 0.16）
//   - DSC_COMPACTION_BASIC_RETAIN_MIN      保留尾部最少 token 数（默认 1024）
//   - DSC_COMPACTION_BASIC_LLM_TIMEOUT     单轮摘要的总时间预算（秒，默认取宿主分发预算
//     DSC_EVENT_DISPATCH_TIMEOUT 减 10s，即默认 20s；必须小于宿主分发预算，否则辛苦算出来
//     的摘要会被超时丢弃）。本地引擎要「一轮压完」而不必分十几轮时，把它与宿主分发预算
//     一起放大（例如两者分别 110 / 120）。
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	dsc "dsc-sdk"

	"dsc/proto"
)

// llmChat 聚合 LLM 的最小接口（宿主 interconnect 的 *llmclient.Client 满足；单测以
// fake 注入）。这里刻意只声明接口、不 import core 的 llmclient 子包：插件只依赖
// dsc-sdk（AGENTS.md §10.5），少一个 core 子包就少一处随内核重构漂移的面。
type llmChat interface {
	Chat(ctx context.Context, messages []*proto.Message, maxTokens int32) (*proto.ChatResponse, error)
}

// compactionRecord 一段已压缩前缀：[0, UpTo) 被替换为 Summary。
// HeadHash 是前缀指纹——历史 append-only 增长时指纹稳定，复用摘要免重复压缩；
// 历史被改写（指纹失配）则全部作废。记录连续覆盖 [0, last.UpTo)。
//
// 后四个字段只用于审计（对齐 DSH compaction/summary 里记 provider/model/usage 的意图：
// 「这段摘要是什么时候、用哪个模型压的、压了多少」要能从落盘记录里直接回答），
// 不参与任何判定。
type compactionRecord struct {
	UpTo      int    `json:"up_to"`           // 已压缩前缀长度（消息条数）
	HeadHash  string `json:"head_hash"`       // 前缀指纹（sha256 hex）
	Summary   string `json:"summary"`         // 摘要内容
	Shadowed  int    `json:"shadowed_tokens"` // 被遮蔽内容的估算 token 数
	ID        string `json:"compaction_id"`   // 稳定标识（时间戳，进程内唯一）
	Covered   int    `json:"covered_messages,omitempty"`
	Model     string `json:"model,omitempty"`  // 写这条摘要的模型（DSC_MODEL）
	Source    string `json:"source,omitempty"` // "llm" | "truncated"（LLM 不可用时的退化摘要）
	CreatedMS int64  `json:"created_ms,omitempty"`
}

// compactionState per-session 压缩状态（进程内 map 项；磁盘副本见 compaction-state.go）。
type compactionState struct {
	Records []compactionRecord `json:"records"`
}

// compactionBasicServer 基础压缩驻留（对齐宿主原 BasicCompactionEngine 语义：
// 压力驱动 + 溢出紧急，保留尾部，LLM 摘要，截断式退化，防重入）。
type compactionBasicServer struct {
	mu         sync.Mutex
	llm        llmChat // interconnect 聚合 LLM（未互联为 nil → 截断式退化）
	compacting bool    // 防重入：同一时间只允许一次压缩

	window      int           // 上下文窗口 token 数（0 = 驻留禁用）
	threshold   float64       // 压力触发阈值比例
	retainRatio float64       // 保留尾部比例
	retainMin   int           // 保留尾部最少 token 数
	llmBudget   time.Duration // 单轮摘要的总时间预算（须小于宿主事件分发预算）

	// inTPS / outTPS 是摘要请求的吞吐估计（token/s，须持 mu）：输入侧（prefill）与输出
	// 侧（decode）。种子取本地引擎的量级，每次调用按实测耗时修正——块大小与输出上限都由
	// 它推出（见 planChunk），所以「第一步就撞满预算」这种单向故障不会再自我复制。
	inTPS  float64
	outTPS float64

	// lastShrinkWarn 上一次「摘要超出预算」告警的时间（限流用，须持 mu）。
	lastShrinkWarn time.Time

	states map[string]*compactionState // per-session 压缩记录（进程内，须持 mu）

	// store 摘要缓存的落盘通道（nil = 纯内存：测试与未装配时走这条）。
	// 由 main 装配注入 dsc.SessionDir()——测试因此不会污染用户的 sessions/。
	store *compactionStore

	// logWindowOnce 首次 pre-step 打印生效参数一次（启动期 stderr 尚未被宿主转发）。
	logWindowOnce sync.Once
}

// 摘要分块与限时的常量（理由见文件头与 compactRegion/planChunk）。
const (
	// defaultSummarizeBudget 单轮摘要的默认总时间预算。宿主的 pre-step 分发是硬超时
	// （见 core/notify.go 的 eventDispatchTimeout），超时后它按「未改写」发出全量历史
	// ——摘要白算、改写被丢。默认取宿主预算减 10s（留给序列化、传输与宿主收口）；
	// 宿主预算被放大时这里自动跟随，不必两处手调。
	defaultSummarizeBudget = 20 * time.Second

	// summaryDispatchReserve 摘要预算与宿主分发预算之间要留的余量。
	summaryDispatchReserve = 10 * time.Second

	// summaryChunkTokens 单块摘要的**上限**（token）。它是一个天花板，不是每次的目标：
	// 实际块大小由 planChunk 按「剩余预算 × 实测吞吐」算出来（慢引擎只有几千 token）。
	// 50K 这个值只对「prefill 几万 tok/s」的云端模型有意义。
	summaryChunkTokens = 50_000

	// summaryChunkTokensMin 单块摘要的下限（token）。再小就切碎消息了，且摘要质量
	// 与输入篇幅强相关（一句话的输入换不来有用的摘要）。
	summaryChunkTokensMin = 1_000

	// summaryMaxTokens 单次摘要的输出上限（token）——同样是天花板，实际值见 planChunk。
	// 按窗口/8 给（256K 窗口 → 32K）会让模型啰嗦到几十秒；摘要的目的是「压缩」。
	summaryMaxTokens = 2048

	// summaryMinTokens 单次摘要的输出下限：低于这个篇幅的摘要不含决策与当前任务。
	summaryMinTokens = 256

	// 吞吐估计的种子（token/s）。取本地引擎的实测量级：prefill ~1.2K tok/s、
	// decode ~30 tok/s。**宁可第一轮小一点**——块太大会撞满预算、一块也压不出来，
	// 而块偏小只是多压几轮（每轮都真的产出一条记录）。云端模型第一轮成功后按实测放大。
	seedSummaryInputTPS  = 1200.0
	seedSummaryOutputTPS = 30.0

	// 吞吐估计的夹取范围：防一次异常耗时（被外界暂停、调度抖动）把估计推到不可用。
	minSummaryInputTPS   = 100.0
	maxSummaryInputTPS   = 200_000.0
	minSummaryOutputTPS  = 3.0
	maxSummaryOutputTPS  = 500.0
	summarySpeedGrowMax  = 1.5 // 单轮最多放大到 1.5×（收敛，不跳变）
	summarySpeedShrinkBy = 0.5 // 撞预算即减半
)

// newCompactionBasicServer 读 env 构建驻留（fail-soft：非法配置逐项回退默认；
// window<=0 即禁用——与 timeout/spill 的 0 禁用同款语义）。
func newCompactionBasicServer() *compactionBasicServer {
	s := &compactionBasicServer{
		window:      131072,
		threshold:   0.70,
		retainRatio: 0.16,
		retainMin:   1024,
		llmBudget:   derivedSummarizeBudget(),
		inTPS:       seedSummaryInputTPS,
		outTPS:      seedSummaryOutputTPS,
	}
	// 窗口来源优先级：DSC_COMPACTION_BASIC_CONTEXT_WINDOW（插件专属）>
	// DSC_CONTEXT_WINDOW（宿主按配置 context_window / 端点探测下发的有效值）> 128K 默认。
	// 此前只读专属键，云端 1M 窗口仍按 128K 判压力、几步就触发压缩；宿主现已把通用键
	// 注入所有插件，这里补回退（显式专属键仍可覆盖）。
	if n := dsc.ContextWindow(); n > 0 {
		s.window = n
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			s.window = n
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 && f < 1 {
			s.threshold = f
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_RETAIN_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f > 0 && f < 1 {
			s.retainRatio = f
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_RETAIN_MIN"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 {
			s.retainMin = n
		}
	}
	if v := os.Getenv("DSC_COMPACTION_BASIC_LLM_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			s.llmBudget = time.Duration(n) * time.Second
		}
	}
	s.states = map[string]*compactionState{}
	return s
}

// derivedSummarizeBudget 从宿主的分发预算推出本插件的单轮摘要预算。
//
// 两者必须同源：宿主预算（DSC_EVENT_DISPATCH_TIMEOUT，默认 30s）决定「这一步最多能花
// 多久」，摘要必须在这之内跑完，否则改写会被超时丢弃（白算）。所以放大宿主预算时，
// 摘要预算自动跟随，少 10s 留给序列化、gRPC 与宿主收口。
func derivedSummarizeBudget() time.Duration {
	v := strings.TrimSpace(os.Getenv("DSC_EVENT_DISPATCH_TIMEOUT"))
	if v == "" {
		return defaultSummarizeBudget
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return defaultSummarizeBudget
	}
	b := time.Duration(n)*time.Second - summaryDispatchReserve
	if b < 5*time.Second {
		b = 5 * time.Second
	}
	return b
}

// attachLLM 缓存 interconnect 聚合 LLM 客户端（宿主挂载后回调注入）。
// nil 不覆盖：未互联时保持 nil，摘要走截断式退化（对齐宿主引擎 nil-LLM 路径）。
//
// **接口里的 typed nil 不是 nil**：宿主在 agent 注册之前就给插件做互通握手时，
// ic.LLM() 返回的是 (*llmclient.Client)(nil)，装进 llmChat 接口后 `c == nil` 为
// false，下一次 summarize 就 nil deref——实测症状是「到达阈值的那一刻插件 panic
// 退出，压缩永远不生效，上下文一路涨到滑窗」，而从会话日志上看只是「没压缩」。
// 判空必须穿透接口看指针本身。
func (s *compactionBasicServer) attachLLM(c llmChat) {
	if c == nil {
		return
	}
	if v := reflect.ValueOf(c); v.Kind() == reflect.Ptr && v.IsNil() {
		return
	}
	s.mu.Lock()
	s.llm = c
	s.mu.Unlock()
}

// handleHostEvent 事件入口（main.go 的 hook 多路复用之一）。
func (s *compactionBasicServer) handleHostEvent(ctx context.Context, eventType, dataJSON string) (string, error) {
	switch eventType {
	case string(dsc.EventAgentPreStep):
		return s.handlePreStep(ctx, dataJSON)
	default:
		return "", nil
	}
}

// handlePreStep 压力驱动压缩（agent/pre-step 拦截）。
// 返回 {"messages": [...]}（改写后列表）或 ""（本步不改写，零开销）。
func (s *compactionBasicServer) handlePreStep(ctx context.Context, dataJSON string) (string, error) {
	if s.window <= 0 {
		return "", nil // 驻留禁用
	}
	// 首次 pre-step 打印实际生效的压缩参数（此时 gRPC 已握手，stderr 由宿主转发到日志）。
	// 用于核对云端 1M 窗口：应看到 window=1048576、阈值=0.70 → 约 734k tokens 才触发。
	s.logWindowOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "dsc-system: compaction-basic window=%d threshold=%.2f retain_ratio=%.2f budget=%s\n",
			s.window, s.threshold, s.retainRatio, s.llmBudget)
	})
	var ev dsc.AgentPreStepEvent
	if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
		return "", fmt.Errorf("compaction-basic: parse pre-step event: %w", err)
	}
	var msgs []*proto.Message
	if ev.MessagesJSON != "" {
		if err := json.Unmarshal([]byte(ev.MessagesJSON), &msgs); err != nil {
			return "", fmt.Errorf("compaction-basic: parse messages: %w", err)
		}
	}
	// 轻载荷：宿主只把消息列表发给**持有压缩岗位**的插件，其余插件收到不带消息的
	// 信封（副作用用 session / user_input 就够）。没有消息就什么都不该做——尤其不能
	// 建/写压缩记录（那等于让一个不在值班的插件改写自己的状态）。
	if len(msgs) == 0 {
		return "", nil
	}
	sess := s.sessionKey(ev.Session)
	s.mu.Lock()
	st := s.stateFor(sess)
	// 指纹校验：历史被改写/回滚时丢弃记录从头评估（重启后从磁盘读回的那份也走这一步）
	records := validRecords(msgs, st.Records)
	pruned := len(records) != len(st.Records)
	if pruned {
		st.Records = records
	}
	s.mu.Unlock()
	if pruned {
		// 作废的记录连同磁盘副本一起清掉：否则下次启动会把它读回来、再白算一遍指纹。
		s.persistState(sess, records)
	}

	// 复用既有压缩后的用量评估：低于阈值则直接返回改写（或无记录时不改写）
	rewritten := applyRecords(msgs, records)
	// 压力 = **我们即将发出的那个表面**的大小（message + 工具目录）。这不是可选项：
	// 压缩后视图只剩「摘要 + 保留尾部」，若拿原始列表（或按原始列表算出的宿主投影）
	// 当压力，压完永远是 900k → 每步再压一次（实测：渲染后仅 2 条消息仍触发第二次
	// 摘要，等于把阈值退化成「保留区一满就压」）。
	//
	// 所以分两种情况，各自用有效的那个数：
	//   - 尚无压缩记录：我们的表面 == 请求表面 → 用宿主投影（ev.ProjectedTokens，
	//     含工具目录、含信封可比性判据），比本地估算更准；
	//   - 已有压缩记录：表面已被我们改写，宿主看不到 → 自测改写后的表面。
	// 无论哪条，词元估算都走 SDK 唯一实现（不新增公式）。
	// 工具目录开销：优先用宿主直接给出的 tool_tokens。旧宿主没有该字段时才反推
	// （TokenCount - 本地消息估算）——同一条瀑布里若有别的插件先改写过消息，
	// 这个减法会把差额算成工具开销，甚至算成负数。
	toolTokens := ev.ToolTokens
	if toolTokens <= 0 {
		toolTokens = ev.TokenCount - dsc.EstimateProtoMessagesTokens(msgs)
		if toolTokens < 0 {
			toolTokens = 0
		}
	}
	var pressure int
	switch {
	case len(records) > 0:
		pressure = dsc.EstimateProtoMessagesTokens(rewritten) + toolTokens
	case ev.ProjectedTokens > 0:
		pressure = ev.ProjectedTokens
	case ev.TokenCount > 0:
		pressure = ev.TokenCount
	default:
		pressure = dsc.EstimateProtoMessagesTokens(rewritten) + toolTokens
	}
	if pressure < s.pressureThreshold() {
		if len(records) > 0 {
			return rewriteJSON(rewritten)
		}
		return "", nil
	}

	// 保留尾部预算：从末尾向前累积（对齐宿主引擎语义：Ratio 与 Min 取大）
	retainIdx := s.retainBoundary(msgs)
	base := 0
	// v3 head 不变量：node 0（system/message）永不被压缩
	// （对齐 DSH 'node 0 is never inside a compaction range'）
	if len(msgs) > 0 && msgs[0].GetRole() == "system" {
		base = 1
	}
	if len(records) > 0 {
		if r := records[len(records)-1].UpTo; r > base {
			base = r
		}
	}
	if retainIdx >= len(msgs) || retainIdx <= base {
		// 可压缩段已被既有记录覆盖（或全部落在保留区）：尽力返回当前改写。
		// 仍然超窗时由宿主滑窗兜住（provider 4xx → 收紧预算 → 重试），本插件不再
		// 做有损的紧急压缩。
		if len(records) > 0 {
			return rewriteJSON(rewritten)
		}
		return "", nil
	}

	recs := s.compactRegion(ctx, msgs, base, retainIdx)
	if len(recs) == 0 {
		// 这一段没压成（摘要超时 / 防重入）：返回当前视图，超窗交给宿主滑窗兜住。
		// 绝不能在这里退化成截断式摘要——那会把没摘要过的历史换成一句话永久落盘。
		if len(records) > 0 {
			return rewriteJSON(rewritten)
		}
		return "", nil
	}
	// 先更新内存、再落盘（顺序无关紧要，但快照必须在解锁后使用：此前这里在解锁后
	// 直接读 st.Records，与同会话的其他 pre-step 存在数据竞争）。
	s.mu.Lock()
	st.Records = append(records, recs...)
	saved := append([]compactionRecord(nil), st.Records...)
	s.mu.Unlock()
	// 落盘是尽力而为：写失败只影响「重启后要不要重压」，不影响本次请求。
	s.persistState(sess, saved)
	return rewriteJSON(applyRecords(msgs, saved))
}

// persistState 把该会话的记录快照写到插件自己的状态文件（store 未装配时纯内存）。
func (s *compactionBasicServer) persistState(sess string, records []compactionRecord) {
	if s.store == nil {
		return
	}
	s.store.Save(sess, records)
}

// compactRegion 把 [base, end) 压成一条或多条记录：按块摘要，每块独立成记录
// （记录即「已压缩前缀」的连续覆盖，多块就是多段摘要）。
//
// 分块 + 限时的理由见文件头。三个硬约束都体现在这里：
//   - 每块的输入/输出预算由 planChunk 按「剩余预算 × 实测吞吐」推出，任何速度的模型
//     都能在预算内跑完第一块（定死 50K 块的做法在本地引擎上必然超时，见文件头）；
//   - 整轮只在 llmBudget 内跑，跑不完就停在已完成的块上——前缀已经缩短，下一轮
//     从新的 base 继续，绝不为了「一次压完」把没摘要的历史换成截断式摘要；
//   - 一块撞了预算就把吞吐估计减半：下一轮的块一定更小。**不能**用同样的块再撞一次
//     同一堵墙（现场：同一个 40,331 token 的块被重发 144 次，全部超时，一次没压成）。
//
// 返回空表示本轮没压成（LLM 忙 / 超时且一块都没完成）：调用方返回既有视图，
// 超窗由宿主滑窗兜住（无损，只影响这一次请求）。
func (s *compactionBasicServer) compactRegion(ctx context.Context, msgs []*proto.Message, base, end int) []compactionRecord {
	s.mu.Lock()
	llm := s.llm
	busy := s.compacting
	if llm != nil && !busy {
		s.compacting = true
	}
	s.mu.Unlock()

	if llm == nil {
		// 未互联：截断式摘要整段替换（本来就只有这一步可做，分段没有意义）。
		return []compactionRecord{s.newRecord(msgs, base, end, truncateSummary(msgs[base:end]), "truncated")}
	}
	if busy {
		// 防重入：上一次摘要还没回来（同一进程极少发生，但发生了就宁可本轮不压）。
		return nil
	}
	defer func() {
		s.mu.Lock()
		s.compacting = false
		s.mu.Unlock()
	}()

	deadline := time.Now().Add(s.llmBudget)
	var out []compactionRecord
	for cur := base; cur < end; {
		left := time.Until(deadline)
		if left <= 0 {
			break // 预算用完：已完成的块照常落库，剩下的下一轮继续
		}
		inTok, outTok := s.planChunk(left)
		next := s.chunkEnd(msgs, cur, end, inTok)
		if next <= cur {
			break
		}
		chunkCtx, cancel := context.WithDeadline(ctx, deadline)
		t0 := time.Now()
		summary, err := s.llmSummary(chunkCtx, llm, msgs[cur:next], outTok)
		elapsed := time.Since(t0)
		cancel()
		if err == nil && summary != "" {
			s.learnSummarySpeed(inTok, outTok, elapsed, true)
			out = append(out, s.newRecord(msgs, cur, next, summary, "llm"))
			cur = next
			continue
		}
		// 宿主已经撤了这个事件（ctx 取消）或预算耗尽：已完成的块落库，剩下的下轮继续。
		// 注意这里**不**退化成截断式摘要——那正是「15 万 token 换一句话」的来源。
		if len(out) > 0 || ctx.Err() != nil || !time.Now().Before(deadline) {
			// 撞了预算就把估计减半：这块在预算内没跑完是**上界实证**，下一轮必须更小。
			// 宿主自己撤了事件时不算（那不是速度问题）。
			if ctx.Err() == nil {
				s.learnSummarySpeed(inTok, outTok, elapsed, false)
				s.warnSlowSummary(inTok, next-cur)
			}
			break
		}
		// 第一块就失败且不是超时：LLM 路径不可用（未就绪 / 被拒），整段走截断式，
		// 与从前的「压缩尽力而为」语义一致。
		return []compactionRecord{s.newRecord(msgs, base, end, truncateSummary(msgs[base:end]), "truncated")}
	}
	return out
}

// planChunk 按「剩余时间 + 当前吞吐估计」给出这一块的输入/输出预算。
//
// 模型：一次摘要的耗时 ≈ inTokens/inTPS + outTokens/outTPS。预算对半分给两侧——块越大
// 压得越多、输出越大摘要越完整，两者都要时间；对慢引擎 decode 往往是更贵的那一半
// （30 tok/s 时 1K 输出就要 33s）。
//
// 这是「同一份预算在不同引擎上都成立」的关键：云端模型算出来就是 50K 块 + 2K 输出
// （与从前的固定值一致），本地引擎算出来是几千 token 块 + 两三百 token 输出——两者
// 都能在预算内跑完，而后者至少**能真的压出一条记录**。
func (s *compactionBasicServer) planChunk(remaining time.Duration) (inTok, outTok int) {
	inTPS, outTPS := s.throughput()
	usable := remaining - remaining/10 // 留一成给序列化、gRPC 与宿主收口
	if usable < time.Second {
		usable = time.Second
	}
	share := (usable / 2).Seconds()
	inTok = int(share * inTPS)
	outTok = int(share * outTPS)
	if inTok < summaryChunkTokensMin {
		inTok = summaryChunkTokensMin
	}
	if inTok > summaryChunkTokens {
		inTok = summaryChunkTokens
	}
	if outTok < summaryMinTokens {
		outTok = summaryMinTokens
	}
	if outTok > summaryMaxTokens {
		outTok = summaryMaxTokens
	}
	return inTok, outTok
}

// throughput 读取当前吞吐估计（须持 mu）。
func (s *compactionBasicServer) throughput() (in, out float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inTPS, s.outTPS
}

// learnSummarySpeed 按一次调用的实测耗时修正吞吐估计。
//
// 失败（撞预算）→ 两侧一起减半：这是「按当前估计一定跑不完」的实证，下一轮必须更小，
// 否则就是「同一块撞同一堵墙」（现场 144 次同样大小、同样超时）。
// 成功 → 按「模型预期耗时 / 实测耗时」放大，单轮最多 1.5×（收敛而非跳变）；贴着预算
// 完成则略收一点，给下一次留余量。
func (s *compactionBasicServer) learnSummarySpeed(inTok, outTok int, elapsed time.Duration, ok bool) {
	// 计时下限：Windows 上 time.Since 的分辨率足以让「秒回」的调用取到 0，整段命中
	// 前缀缓存时也接近 0。按 0 直接丢弃这条实证，冷启动的保守种子就永远放不大
	// （云端模型会一直用本地引擎量级的小块去压）。
	if elapsed < time.Millisecond {
		elapsed = time.Millisecond
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	scale := 1.0
	if !ok {
		scale = summarySpeedShrinkBy
	} else if model := float64(inTok)/s.inTPS + float64(outTok)/s.outTPS; model > 0 {
		actual := elapsed.Seconds()
		switch {
		case actual*3 < model: // 明显比估计快：放大（上限 1.5×/轮）
			scale = model / actual
			if scale > summarySpeedGrowMax {
				scale = summarySpeedGrowMax
			}
		case actual*1.2 > model: // 贴着预算完成：略收，别把下一次推到超时
			scale = 0.9
		}
	}
	s.inTPS = clampFloat(s.inTPS*scale, minSummaryInputTPS, maxSummaryInputTPS)
	s.outTPS = clampFloat(s.outTPS*scale, minSummaryOutputTPS, maxSummaryOutputTPS)
}

// warnSlowSummary 打印一次「摘要跑不完预算」的告警（30s 限流，避免每步刷屏）。
//
// 这行日志是这类故障唯一的外部可见证据：不打印时，用户只能看到「过了阈值却一直没压缩」，
// 而引擎侧看到的只是「客户端断开」。它同时给出下一步的自愈动作与可调项。
func (s *compactionBasicServer) warnSlowSummary(inTok, msgCount int) {
	s.mu.Lock()
	if !s.lastShrinkWarn.IsZero() && time.Since(s.lastShrinkWarn) < 30*time.Second {
		s.mu.Unlock()
		return
	}
	s.lastShrinkWarn = time.Now()
	inTPS, outTPS := s.inTPS, s.outTPS
	budget := s.llmBudget
	s.mu.Unlock()
	fmt.Fprintf(os.Stderr, "dsc-system: compaction-basic: 摘要超出单轮预算（%s 内没跑完 ~%d token / %d 条消息的块），"+
		"吞吐估计降到 %.0f tok/s 输入 / %.0f tok/s 输出，下一块会更小。"+
		"本地引擎想一轮压完（而不是分十几轮），把 DSC_EVENT_DISPATCH_TIMEOUT 与 "+
		"DSC_COMPACTION_BASIC_LLM_TIMEOUT 一起放大（如 120 / 110）\n",
		budget, inTok, msgCount, inTPS, outTPS)
}

// clampFloat 夹取（避免为一行夹取再引入 math）。
func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// newRecord 组装一条压缩记录（一条记录 = 一段被摘要替换的前缀）。
func (s *compactionBasicServer) newRecord(msgs []*proto.Message, from, to int, summary, source string) compactionRecord {
	now := time.Now()
	return compactionRecord{
		UpTo:     to,
		HeadHash: fingerprint(msgs[:to]),
		Summary:  summary,
		// 同一毫秒内可能产出多条（小块连压），id 带上界避免撞号。
		ID:        fmt.Sprintf("compact-%d-%d", now.UnixMilli(), to),
		Shadowed:  dsc.EstimateProtoMessagesTokens(msgs[from:to]),
		Covered:   to - from,
		Model:     dsc.EnvStr("DSC_MODEL", ""),
		Source:    source,
		CreatedMS: now.UnixMilli(),
	}
}

// chunkEnd 返回本块的上界：从 from 起累加到「再加一条就超额」为止（至少一条）。
//
// limit 是 planChunk 按剩余预算与实测吞吐算出来的输入预算，不是常量：一次摘要能读
// 多少由「这段时间内模型能读完多少」决定，与窗口无关（区间长度本就被窗口约束：
// ≤ 窗口 − 保留区）。
func (s *compactionBasicServer) chunkEnd(msgs []*proto.Message, from, end, limit int) int {
	if limit <= 0 {
		limit = summaryChunkTokensMin
	}
	acc := 0
	for i := from; i < end; i++ {
		acc += dsc.EstimateProtoMessageTokens(msgs[i])
		if acc > limit {
			if i == from {
				return from + 1 // 单条就超预算：只能自成一块，再切就切碎消息了
			}
			return i
		}
	}
	return end
}

// llmSummary 构建压缩请求并调用聚合 LLM（提示词与宿主原引擎逐字一致）。
// maxTokens 由 planChunk 按剩余预算与输出吞吐给出。
func (s *compactionBasicServer) llmSummary(ctx context.Context, llm llmChat, region []*proto.Message, maxTokens int) (string, error) {
	prompt := "你是对话压缩器。请将下面的对话历史压缩成一段精简但信息完整的摘要。\n" +
		"压缩优先级：① 永远先保留用户【当前正在推进的最紧要任务/事项】——压缩后模型仍须明确知道" +
		"现在要做什么、卡在哪、下一步是什么；② 保留重要决策、关键结论、工具结果与文件路径等后续仍要用的事实；" +
		"③ 早期已完成、与当前任务不再相关的内容可大幅精简甚至丢弃。不要因为某条消息「位置靠前」就保留它——" +
		"在超长会话里那往往是很久以前的失效旧需求。只输出摘要，不添加额外解释。\n\n--- 对话历史 ---\n"
	for _, msg := range region {
		prompt += fmt.Sprintf("[%s] %s\n", msg.GetRole(), msg.GetContent())
	}
	compactMsgs := []*proto.Message{
		{Role: "system", Content: prompt},
		{Role: "user", Content: "请压缩上述对话历史。"},
	}
	// 输出上限由调用方按「剩余预算 × 输出吞吐」给出（planChunk）；这里只做兜底夹取，
	// 防止非法值把输出预算推到离谱的量级。
	if maxTokens > summaryMaxTokens {
		maxTokens = summaryMaxTokens
	}
	if maxTokens < summaryMinTokens {
		maxTokens = summaryMinTokens
	}
	resp, err := llm.Chat(ctx, compactMsgs, int32(maxTokens))
	if err != nil {
		return "", fmt.Errorf("compaction-basic LLM call failed: %w", err)
	}
	if resp.GetContent() == "" {
		return "", fmt.Errorf("compaction-basic returned empty summary")
	}
	return resp.GetContent(), nil
}

// pressureThreshold 压力触发阈值（token 数）。
func (s *compactionBasicServer) pressureThreshold() int {
	return int(float64(s.window) * s.threshold)
}

// retainBudget 保留尾部预算（token 数）：Ratio 与 Min 取大（对齐宿主引擎）。
func (s *compactionBasicServer) retainBudget() int {
	b := int(float64(s.window) * s.retainRatio)
	if b < s.retainMin {
		b = s.retainMin
	}
	return b
}

// retainBoundary 保留尾部边界：从末尾向前累积，返回前段可压缩的上界
// （[retainIdx, len) 为保留区）。全部落在保留区时返回 len(msgs)。
// 用量估算复用 core 公用启发式（dsc.EstimateProtoMessageTokens，CJK 感知
// + 角色开销 + 图像 + 工具参数），与压力判定同一估算口径。
func (s *compactionBasicServer) retainBoundary(msgs []*proto.Message) int {
	budget := s.retainBudget()
	acc := 0
	// v3 head 不变量：node 0（system/message）永不被压缩——
	// 迭代下界为 1（跳过 system），避免极端小预算时 system 落入压缩区。
	lower := 0
	if len(msgs) > 0 && msgs[0].GetRole() == "system" {
		lower = 1
	}
	for i := len(msgs) - 1; i >= lower; i-- {
		acc += dsc.EstimateProtoMessageTokens(msgs[i])
		if acc > budget {
			return s.snapToPairBoundary(msgs, i+1)
		}
	}
	return len(msgs)
}

// snapToPairBoundary 把「保留区起点」推到不切断 tool_call ↔ tool_result 的位置。
//
// 预算边界是纯按 token 累积出来的，可能正好落在 assistant(tool_calls) 与其
// tool_result 之间：调用被压进摘要、结果留在保留区，provider 收到孤立的 tool 消息
// 会直接 400（整轮失败）。这里把落单的结果一并划进压缩区（起点后移），保留区略微
// 变小，但配对完整——这是硬要求，不是优化。
func (s *compactionBasicServer) snapToPairBoundary(msgs []*proto.Message, idx int) int {
	for idx < len(msgs) && msgs[idx].GetRole() == "tool" {
		idx++
	}
	if idx >= len(msgs) {
		return len(msgs)
	}
	return idx
}

// truncateSummary 截断式退化摘要：取首条+末条拼接（对齐宿主引擎 truncateCompact，
// 溢出安全：单侧内容截断到 2000 字符，防截断摘要本身仍超预算）。
//
// 这是**压缩**路径：用摘要替换整段历史、明确标着 `[压缩摘要]`，与滑窗是两件事——滑窗
// 保留原始消息、只在开头加一行「已省略 N 条」，并不改写任何正文。这里保留区间首尾各一句：
// 这段是要被替换掉的历史，模型只能靠摘要知道「这里发生过什么」，给出当初的话题与最近的
// 状态比一句计数有用。
func truncateSummary(region []*proto.Message) string {
	if len(region) == 0 {
		return ""
	}
	summary := "[压缩摘要] " + capText(region[0].GetContent())
	if len(region) > 1 {
		summary += "\n...\n" + capText(region[len(region)-1].GetContent())
	}
	return summary
}

// truncateCap 截断式摘要单侧内容上限（字符）。
const truncateCap = 2000

// capText 按字符安全截断（防 CJK 拆字）。
func capText(s string) string {
	r := []rune(s)
	if len(r) > truncateCap {
		return string(r[:truncateCap]) + "…[截断]"
	}
	return s
}

// applyRecords 把已压缩记录应用到消息列表：前缀替换为逐条摘要（user 角色，
// 显式标注 compaction-basic 标识），其余原样保留。记录失配/越界时原样返回。
func applyRecords(msgs []*proto.Message, records []compactionRecord) []*proto.Message {
	if len(records) == 0 {
		return msgs
	}
	last := records[len(records)-1]
	if last.UpTo <= 0 || last.UpTo > len(msgs) {
		return msgs
	}
	out := make([]*proto.Message, 0, len(records)+len(msgs)-last.UpTo)
	// v3 head 不变量：若 msgs[0] 是 system 消息，原样保留——
	// UpTo 的 base 已确保 >= 1（见 handlePreStep/handleRequestError）
	if len(msgs) > 0 && msgs[0].GetRole() == "system" {
		out = append(out, msgs[0])
	}
	for _, rec := range records {
		marker := "[上下文压缩摘要 compaction-basic=" + rec.ID + "]"
		out = append(out, &proto.Message{Role: "user", Content: marker + "\n" + rec.Summary})
	}
	out = append(out, msgs[last.UpTo:]...)
	return out
}

// validRecords 校验既有记录与当前消息列表的指纹吻合性：记录连续覆盖 [0, last.UpTo)，
// 仅最后一条的前缀指纹需要复核。失配（历史被改写/回滚/换会话）时全部作废。
func validRecords(msgs []*proto.Message, records []compactionRecord) []compactionRecord {
	if len(records) == 0 {
		return nil
	}
	last := records[len(records)-1]
	if last.UpTo > 0 && last.UpTo <= len(msgs) && fingerprint(msgs[:last.UpTo]) == last.HeadHash {
		return records
	}
	return nil
}

// fingerprint 前缀指纹：role + NUL + content + NUL 串联的 sha256 hex。
func fingerprint(msgs []*proto.Message) string {
	h := sha256.New()
	for _, m := range msgs {
		h.Write([]byte(m.GetRole()))
		h.Write([]byte{0})
		h.Write([]byte(m.GetContent()))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// rewriteJSON 组装 pre-step 改写结果（宿主 applyPreStepHook 协议：
// {"messages": [...]}，proto.Message 数组 JSON）。
func rewriteJSON(msgs []*proto.Message) (string, error) {
	b, err := json.Marshal(msgs)
	if err != nil {
		return "", err
	}
	return `{"messages": ` + string(b) + `}`, nil
}

// stateFor per-session 压缩记录（须持 s.mu；缺失即从磁盘读一份，再退化为空状态）。
//
// 这里做一次读盘：每个会话只在首次 pre-step 发生一次，换来的是「重启后不必把同一段
// 前缀重新摘要一遍」。store 未装配（测试）时纯内存，行为与从前一致。
func (s *compactionBasicServer) stateFor(sessID string) *compactionState {
	if st, ok := s.states[sessID]; ok {
		return st
	}
	st := &compactionState{}
	if s.store != nil {
		st.Records = s.store.Load(sessID)
	}
	s.states[sessID] = st
	return st
}

// sessionKey 会话状态键：优先事件透传的 session_id（宿主 ChatRequest.session_id），
// 缺省按工作区派生项目级键（SDK 的 ProjectSessionKey，与宿主 session 存储同一实现——
// 同一项目同名、不同项目隔离）。
func (s *compactionBasicServer) sessionKey(sessID string) string {
	if sessID == "" {
		return dsc.ProjectSessionKey()
	}
	return sessID
}
