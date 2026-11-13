package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"dsc/jobs"
	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// 事件分发的单次预算。
//
// 30s 对「一步一次的小事件」绰绰有余，但 agent/pre-step 里挂的是**压缩后端**：它要在这
// 一步内真的跑完一次模型摘要，而摘要耗时与模型速度成正比。本地引擎（prefill 1~2K tok/s、
// decode 20~40 tok/s）上，一次 40K 的摘要光读 prompt 就要 25s，于是每一轮都撞满预算、
// 摘要被丢弃——现场表现就是「上下文涨到窗口的 88% 仍然一次没压缩」，而会话日志里一条
// 压缩记录都没有。
//
// 这类部署只能靠放大预算解决，所以开放 DSC_EVENT_DISPATCH_TIMEOUT（秒，5~600）。
// 压缩后端从**同一个**变量推导自己的单轮预算（少 10s 给序列化与收口），两边不会互相踩。
const (
	defaultEventDispatchBudget = 30 * time.Second
	minEventDispatchBudget     = 5 * time.Second
	maxEventDispatchBudget     = 600 * time.Second
)

// eventDispatchTimeout 返回单次事件分发的预算（DSC_EVENT_DISPATCH_TIMEOUT，非法值回默认）。
func eventDispatchTimeout() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DSC_EVENT_DISPATCH_TIMEOUT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			d := time.Duration(n) * time.Second
			if d >= minEventDispatchBudget && d <= maxEventDispatchBudget {
				return d
			}
		}
	}
	return defaultEventDispatchBudget
}

// 插件通知服务（互通机制 2，插件→宿主）：插件进程经 broker 连接本服务，
// 把事件（含后台任务完成通知）发布到宿主事件总线，供 TUI/其他插件订阅。
// 宿主在 broker 挂载并注入 serviceID（DSC_NOTIFY_SERVICE_ID）到插件进程 env。

// servePluginNotifyLocked 在 agent broker 上挂载 PluginNotifyService（需已持有 m.mu）。
func (m *Manager) servePluginNotifyLocked() (uint32, error) {
	id, err := m.servePluginNotifyOnBroker(m.broker)
	if err == nil {
		m.coreNotifyServiceID = id
	}
	return id, err
}

// servePluginNotifyOnBroker 在指定 broker 上挂载 PluginNotifyService；返回
// serviceID。互通机制 2 中，该服务须挂在本插件 client 的 broker 上（插件
// 进程经自身 broker.Dial 访问），而不仅是 agent broker。
func (m *Manager) servePluginNotifyOnBroker(broker *plugin.GRPCBroker) (uint32, error) {
	if broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve core notify service")
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterPluginNotifyServiceServer(s, &coreNotifyServer{m: m})
		return s
	})
	return serviceID, nil
}

// coreNotifyServer PluginNotifyService 宿主侧实现。
type coreNotifyServer struct {
	proto.UnimplementedPluginNotifyServiceServer
	m *Manager
}

// Notify 把插件事件发布到宿主事件总线：
//   - name == job/done 时特化：data 为任务快照 JSON，宿主解析为 JobSnapshot 后
//     发 JobDoneEvent（复用 TUI 完成通知唤醒体系）；
//   - 其余 name 为插件自定义事件，data 尝试 JSON 解析（失败按原样字符串）。
func (s *coreNotifyServer) Notify(ctx context.Context, req *proto.NotifyRequest) (*proto.NotifyResponse, error) {
	if req.GetName() == string(JobDoneEvent) {
		var snap jobs.JobSnapshot
		if err := json.Unmarshal([]byte(req.GetData()), &snap); err != nil {
			return nil, fmt.Errorf("notify: invalid job/done data: %w", err)
		}
		s.m.events.Emit(JobDoneEvent, EventContext{Data: snap})
		return &proto.NotifyResponse{}, nil
	}
	data := any(req.GetData())
	if req.GetData() != "" {
		var v any
		if err := json.Unmarshal([]byte(req.GetData()), &v); err == nil {
			data = v
		}
	}
	s.m.events.Emit(EventName(req.GetName()), EventContext{Data: data})
	return &proto.NotifyResponse{}, nil
}

// hookClientsSnapshot 返回按加载顺序的插件钩子客户端快照（读锁保护，防热加载竞态）。
func (m *Manager) hookClientsSnapshot() []proto.PluginHookServiceClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]proto.PluginHookServiceClient, 0, len(m.toolHookOrder))
	for _, n := range m.toolHookOrder {
		out = append(out, m.toolHookClients[n])
	}
	return out
}

// capPreStepMessages 插件声明「即使不持有压缩岗位，也需要 pre-step 的完整消息列表」。
//
// 默认不发：岗位之外没有任何插件能改写请求面，把几 MB 的消息复制过去没有用处——
// 实测非岗位的压缩后端还会真的调模型做摘要，结果被宿主丢弃。观察型插件（记忆、审计）
// 若确实要看消息，在 SDK Config 的 Provides 里声明这一项即可。
const capPreStepMessages = "pre-step-messages"

// capPreStepTransform 插件声明「我提供一个可组合的请求面变换（projection）」。
//
// 与 compaction 的分工：压缩是「谁当值谁说了算」，只能有一个；投影是可以叠加的
// 若干道工序（按预算退役旧图像、按类型截断大段工具结果……），谁声明谁参与，按装载
// 顺序依次作用在上一步的输出上。早先投影只能寄生在压缩插件内部——于是压缩换人时
// 投影跟着消失，那不是配置问题，是把两件事绑在了一起。
const capPreStepTransform = "pre-step-transform"

// CompactionRole 是「压缩岗位」的解析结果。
//
// 有压缩能力的插件可以有多个，岗位只有一个：谁当值，谁的 pre-step 改写被采纳，
// 也只有它需要拿到完整消息列表。岗位在启动期就能定死（配置点名 + 已加载 + 声明了
// compaction 能力），不必等广播之后再从多个结果里挑。
//
// 压缩是 agent 的**强依赖**（agent 元数据声明 requires/dsc/compaction）：没有当值的
// 后端，agent 不会被激活，也就没有循环可跑。因此「空缺」不再是可接受的常态，而是
// 启动期就会失败的状态——宿主 main 在加载完成后据此拒绝启动。
//
// 不做缓存的理由：插件热加载/卸载会改变答案，缓存得额外维护失效；解析本身只是查两张
// 表，代价可以忽略。
type CompactionRole struct {
	// Holder 岗位持有者（空 = 空缺：没有任何插件能改写请求面，进程不允许运行）。
	Holder string
	// Configured config.yaml 的 compaction 字段（可能指向一个没加载的插件）。
	Configured string
	// Candidates 当前已加载、且声明了 compaction 能力的插件（按名字排序）。
	Candidates []string
	// AutoSelected 只有唯一候选、且配置没点名时，宿主直接把它放上岗位
	// （对齐「启用即生效」：用户只启用一个后端就不必再写一行配置）。
	AutoSelected bool
	// Vacancy 岗位空缺的原因（空 = 有人当值）。空缺时宿主必须把原因说清楚：
	// 缺的是「没启用插件」「点名的没加载」「点名的没声明能力」还是「多候选未点名」。
	Vacancy string
}

// ResolveCompactionRole 解析压缩岗位（唯一入口：改名/换后端只需改这里）。
//
// 选择规则见 pickCompactionProviderLocked：配置点名且合格者当值；未点名时若只有一个
// 候选，它自动当值；多候选且未点名则空缺（要求用户点名，避免两个后端都改写请求面）。
func (m *Manager) ResolveCompactionRole() CompactionRole {
	m.mu.RLock()
	defer m.mu.RUnlock()
	holder, candidates, reason := m.pickCompactionProviderLocked("")
	return CompactionRole{
		Holder:       holder,
		Configured:   m.compactionBackend,
		Candidates:   candidates,
		AutoSelected: m.compactionBackend == "" && holder != "",
		Vacancy:      reason,
	}
}

// pickCompactionProviderLocked 选出压缩能力提供者（需持有锁）。providerType 为空表示
// 不限类型（岗位解析用），非空时只在该类型的插件里找（能力依赖解析用）。
//
// 规则（唯一实现——依赖解析与岗位解析必须给出同一个答案，否则会出现「岗位有人、
// agent 却因依赖未满足而 PENDING」这种自相矛盾的状态）：
//  1. config.yaml 的 compaction 点名且该插件已加载并声明了 compaction → 它；
//  2. 未点名、恰好一个候选 → 它（启用即生效）；
//  3. 其余情况 → 空 + 原因（点名的没加载/没声明，或多候选未点名）。
//
// 多候选为什么不自动取首个：两个后端同时改写请求面会层层叠加，结果既不是 A 也不是 B。
// 显式点名的角色与 LLM 的 default_llm 同构——多实现并存时由配置做唯一选择。
func (m *Manager) pickCompactionProviderLocked(providerType string) (holder string, candidates []string, reason string) {
	for name, info := range m.coreMetadata {
		if info == nil || m.toolHookClients[name] == nil {
			continue // 未加载的不算候选
		}
		if providerType != "" && info.Type != providerType {
			continue
		}
		v, ok := info.Capabilities["compaction"]
		if ok && v != "false" {
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)

	switch {
	case m.compactionBackend != "":
		if m.toolHookClients[m.compactionBackend] == nil {
			return "", candidates, fmt.Sprintf("configured backend %q is not loaded", m.compactionBackend)
		}
		if info := m.coreMetadata[m.compactionBackend]; info == nil || info.Capabilities["compaction"] == "" || info.Capabilities["compaction"] == "false" {
			return "", candidates, fmt.Sprintf("configured backend %q does not declare Provides compaction", m.compactionBackend)
		}
		return m.compactionBackend, candidates, ""
	case len(candidates) == 1:
		return candidates[0], candidates, ""
	case len(candidates) > 1:
		return "", candidates, fmt.Sprintf("multiple compaction candidates (%s) but config.yaml has no `compaction:` to put one on duty", strings.Join(candidates, ", "))
	default:
		return "", nil, "no loaded plugin declares the compaction capability"
	}
}

// declaresCompactionLocked 报告某个已加载插件是否声明了 compaction 能力（需持有读锁）。
func (m *Manager) declaresCompactionLocked(name string) bool {
	info := m.coreMetadata[name]
	if info == nil {
		return false
	}
	v, ok := info.Capabilities["compaction"]
	return ok && v != "false"
}

// wantsPreStepMessages 报告该插件是否显式声明需要 pre-step 的完整消息列表。
func (m *Manager) wantsPreStepMessages(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info := m.coreMetadata[name]
	if info == nil {
		return false
	}
	v, ok := info.Capabilities[capPreStepMessages]
	return ok && v != "false"
}

// compactionBackendName 返回岗位持有者（空 = 岗位空缺，进程不允许运行）。
func (m *Manager) compactionBackendName() string {
	return m.ResolveCompactionRole().Holder
}

// CompactionBackendState 返回配置指定的后端名与岗位是否真的有人当值。
//
// 给前端/诊断用：config.yaml 里写了 `compaction: X` 但插件没加载（plugins 列表里
// 注释掉、enabled: false、二进制缺失）时，岗位为空——没有回落路径，启动会失败并说明
// 原因（见 main 的启动闸门）。
func (m *Manager) CompactionBackendState() (configured string, active bool) {
	role := m.ResolveCompactionRole()
	return role.Configured, role.Holder != ""
}

// CompactionRoleProblem 返回「岗位空缺」的一句话说明（一切正常时返回空串）。
//
// 宿主在插件加载完成后据此拒绝启动：压缩是 agent 的强依赖，空缺不是可选形态。
// 把原因写清楚是本函数的全部职责——用户要能直接看出缺的是「没启用插件」「点名的
// 没加载」还是「多候选未点名」。
func (m *Manager) CompactionRoleProblem() string {
	if m == nil {
		return ""
	}
	role := m.ResolveCompactionRole()
	if role.Holder != "" {
		return ""
	}
	m.logger.Error("compaction role is vacant",
		"configured", role.Configured, "reason", role.Vacancy,
		"candidates", role.Candidates,
		"hint", "enable a plugin that declares Provides compaction in config.yaml (and set `compaction: <name>` when several are enabled)")
	return fmt.Sprintf("no compaction backend on duty (%s); candidates=%v configured=%q",
		role.Vacancy, role.Candidates, role.Configured)
}

// compactionBackendActive 报告岗位是否有人当值。
func (m *Manager) compactionBackendActive() bool {
	return m.ResolveCompactionRole().Holder != ""
}

// LoadLuaHooks 加载外部脚本钩子配置（-hooks 指定的 hooks.json；路径不存在为
// 无配置静默）。加载成功后，外部钩子在工具流水线 BeforeTool/AfterTool 阶段
// 参与裁定（veto/改写），与插件 gRPC 钩子串联（插件先行，外部脚本随后）。
func (m *Manager) LoadLuaHooks(path string) error {
	b := NewLuaHookBridge()
	if err := b.LoadConfig(path); err != nil {
		return err
	}
	m.mu.Lock()
	m.luaHooks = b
	m.mu.Unlock()
	m.logger.Info("lua hooks loaded", "path", path)
	return nil
}

// runPluginBeforeTool 调用所有插件 BeforeTool 钩子（按加载顺序）：任一 veto
// 阻止执行；参数可被改写（后续用新参数）。插件不可用（UNIMPLEMENTED 等）跳过。
// 插件 gRPC 钩子之后串联外部脚本钩子（LuaHookBridge：严格 LUA / 直接 exec），
// 每次调用结算留痕（-log 启用时的钩子诊断能力：谁被调、耗时、veto/改写、失败原因）。
func (m *Manager) runPluginBeforeTool(ctx context.Context, inv *ToolInvocation) error {
	for _, c := range m.hookClientsSnapshot() {
		if c == nil {
			continue
		}
		resp, err := c.BeforeTool(ctx, &proto.BeforeToolRequest{
			ToolName: inv.ToolName, ArgumentsJson: inv.ArgumentsJSON, CallId: inv.CallID,
		})
		if err != nil {
			continue // UNIMPLEMENTED/插件不可用容错
		}
		if resp.GetVeto() {
			if resp.GetError() != "" {
				return fmt.Errorf("core vetoed %s: %s", inv.ToolName, resp.GetError())
			}
			return fmt.Errorf("core vetoed %s", inv.ToolName)
		}
		if resp.GetArgumentsJson() != "" && resp.GetArgumentsJson() != inv.ArgumentsJSON {
			inv.ArgumentsJSON = resp.GetArgumentsJson()
		}
	}
	// 外部脚本钩子（严格 LUA / 直接 exec）：veto 语义与插件钩子一致，失败 contain。
	m.mu.RLock()
	hooks := m.luaHooks
	m.mu.RUnlock()
	if hooks != nil {
		start := time.Now()
		rewritten, err := hooks.RunBeforeTool(ctx, inv.ToolName, inv.ArgumentsJSON, inv.CallID)
		elapsed := time.Since(start).Milliseconds()
		if err != nil {
			// veto：模型可见的阻止原因（对齐 DSH「用模型可见消息阻止工具调用」）
			m.logger.Warn("lua hook vetoed tool", "tool", inv.ToolName,
				"duration_ms", elapsed, "error", err.Error())
			return err
		}
		if rewritten != inv.ArgumentsJSON {
			m.logger.Info("lua hook rewrote args", "tool", inv.ToolName,
				"duration_ms", elapsed, "args_len", len(rewritten))
			inv.ArgumentsJSON = rewritten
		} else {
			m.logger.Info("lua hook passed", "tool", inv.ToolName, "duration_ms", elapsed)
		}
	}
	return nil
}

// runPluginAfterTool 调用所有插件 AfterTool 钩子：可改写结果/错误。随后串联
// 外部脚本钩子（LuaHookBridge），调用结算留痕同 BeforeTool。
func (m *Manager) runPluginAfterTool(ctx context.Context, inv *ToolInvocation) {
	for _, c := range m.hookClientsSnapshot() {
		if c == nil {
			continue
		}
		resp, err := c.AfterTool(ctx, &proto.AfterToolRequest{
			ToolName: inv.ToolName, ArgumentsJson: inv.ArgumentsJSON, CallId: inv.CallID,
			Result: inv.Result, Error: errString(inv.Err),
		})
		if err != nil {
			continue
		}
		if resp.GetError() != "" {
			inv.Err = fmt.Errorf("%s", resp.GetError())
		} else if resp.GetResult() != "" {
			inv.Result = resp.GetResult()
			inv.Err = nil
		}
	}
	// 外部脚本钩子（严格 LUA / 直接 exec）：改写结果/置错，失败 contain。
	m.mu.RLock()
	hooks := m.luaHooks
	m.mu.RUnlock()
	if hooks != nil {
		start := time.Now()
		result, toolErr := hooks.RunAfterTool(ctx, inv.ToolName, inv.ArgumentsJSON,
			inv.Result, errString(inv.Err), inv.CallID)
		elapsed := time.Since(start).Milliseconds()
		switch {
		case toolErr != "" && (inv.Err == nil || toolErr != inv.Err.Error()):
			m.logger.Warn("lua hook set tool error", "tool", inv.ToolName,
				"duration_ms", elapsed, "error", toolErr)
			inv.Err = fmt.Errorf("%s", toolErr)
		case result != inv.Result:
			m.logger.Info("lua hook rewrote result", "tool", inv.ToolName,
				"duration_ms", elapsed, "result_chars", len(result))
			inv.Result = result
			inv.Err = nil
		default:
			m.logger.Info("lua hook passed", "tool", inv.ToolName, "duration_ms", elapsed, "phase", "after")
		}
	}
}

// errString 错误转字符串（nil → 空）。
func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// dispatchEventToPlugins 把宿主事件同步分发到所有插件（互通机制 3 OnEvent）：
// 插件可经此订阅宿主及其他插件（notify 发布）的事件，无需协调发布方。
//
// 分发模式由事件名决定（见 eventDispatchMode 映射表，对齐 DSH Cordis ctx.on()）：
//   - emit（默认）：同步顺序调用所有插件，忽略返回值；插件内部可自行 go func 派发副作用
//   - waterfall：洋葱模型，每个插件可调 next 委托下游，返回非空 result_json 改写载荷
//     透传给下游；返回非空 error 短路链视为 veto
//   - serial/bail：error 非空使链停止（bail 短路返回，serial 错误聚合）
//
// 同步语义：宿主等待所有插件返回后才继续（避免异步竞态）。
// 通知型插件（如 notify）若需异步副作用（如播放音效），自行在 handler 内 go func()。
//
// waterfall 模式下返回 (result, err)：result 为最终改写后的载荷（any 类型，调用方
// 自行类型断言）；err 非 nil 表示链被 veto（含插件返回的 error 文本）。
// namedHookClient 带插件名的钩子客户端（派发时需知道来源才能决定是否采纳其改写）。
type namedHookClient struct {
	name   string
	client proto.PluginHookServiceClient
}

// hookClientsNamedSnapshot 按加载顺序返回（插件名, 客户端）快照，跳过空客户端。
func (m *Manager) hookClientsNamedSnapshot() []namedHookClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]namedHookClient, 0, len(m.toolHookOrder))
	for _, n := range m.toolHookOrder {
		if c := m.toolHookClients[n]; c != nil {
			out = append(out, namedHookClient{name: n, client: c})
		}
	}
	return out
}

// collectPreStepAppends 从插件返回的 result_json 中取出一次性插播消息。
// withPreStepStage 复制一份 pre-step 载荷并写入阶段标记（不改原事件）。
func withPreStepStage(data any, stage string) any {
	switch ev := data.(type) {
	case AgentPreStepEvent:
		ev.Stage = stage
		return ev
	case *AgentPreStepEvent:
		if ev == nil {
			return data
		}
		cp := *ev
		cp.Stage = stage
		return cp
	case map[string]any:
		out := make(map[string]any, len(ev)+1)
		for k, v := range ev {
			out[k] = v
		}
		out["stage"] = stage
		return out
	}
	return data
}

// applyPreStepProjections 跑第二阶段：让每个声明 pre-step-transform 的插件在
// **当前视图**上依次投影，返回合并后的结果。
//
// 约定（写进文档与 sdk 常量注释）：投影是无损变换——不删消息、不改角色顺序、不动
// 工具配对，只改单条消息的内容/附件。这样它才能安全地叠加在压缩结果之上。
//
// 失败处理与第一阶段不同：压缩改写的失败会让请求超窗（必须 veto），而投影只是
// 优化（省下的是图像/长输出的字节）。所以某个投影报错或传输失败时记 WARN 并继续
// 用上一步的结果，绝不因此让整轮请求失败。
func (m *Manager) applyPreStepProjections(ctx context.Context, clients []namedHookClient, data any, result any) any {
	var transforms []namedHookClient
	for _, nc := range clients {
		if m.wantsPreStepTransform(nc.name) {
			transforms = append(transforms, nc)
		}
	}
	if len(transforms) == 0 {
		return result
	}

	msgs := ""
	if raw, ok := extractPreStepMessages(result); ok {
		msgs = raw
	} else if ev, ok := data.(AgentPreStepEvent); ok {
		msgs = ev.MessagesJSON
	}
	if msgs == "" {
		return result // 没有视图可投影（空会话/旧载荷）：不白跑
	}

	changed := false
	for _, nc := range transforms {
		payload, ok := preStepProjectionPayload(data, msgs)
		if !ok {
			m.logger.Warn("pre-step projection skipped: cannot build payload", "plugin", nc.name)
			continue
		}
		resp, err := nc.client.OnEvent(ctx, &proto.OnEventRequest{Name: string(EventAgentPreStep), DataJson: payload})
		if err != nil {
			m.logger.Warn("pre-step projection transport error, keeping previous view",
				"plugin", nc.name, "error", err.Error())
			continue
		}
		if e := resp.GetError(); e != "" {
			m.logger.Warn("pre-step projection failed, keeping previous view", "plugin", nc.name, "error", e)
			continue
		}
		next, ok := extractPreStepMessagesJSON(resp.GetResultJson())
		if !ok || next == msgs {
			continue
		}
		msgs = next
		changed = true
	}
	if !changed {
		return result
	}

	out, _ := result.(map[string]any)
	if out == nil {
		out = map[string]any{}
	}
	var arr any
	if err := json.Unmarshal([]byte(msgs), &arr); err != nil {
		m.logger.Warn("pre-step projection produced invalid messages JSON, ignoring", "error", err.Error())
		return result
	}
	out["messages"] = arr
	return out
}

// preStepProjectionPayload 生成第二阶段的载荷：同一信封，消息换成当前视图，阶段标记
// 为 projection。
func preStepProjectionPayload(data any, messagesJSON string) (string, bool) {
	staged := withPreStepStage(data, PreStepStageProjection)
	switch ev := staged.(type) {
	case AgentPreStepEvent:
		ev.MessagesJSON = messagesJSON
		b, err := json.Marshal(ev)
		return string(b), err == nil
	case map[string]any:
		ev["messages_json"] = messagesJSON
		b, err := json.Marshal(ev)
		return string(b), err == nil
	}
	return "", false
}

// extractPreStepMessages 取结果载荷里的消息数组（原样 JSON 文本）。
func extractPreStepMessages(result any) (string, bool) {
	m, ok := result.(map[string]any)
	if !ok {
		return "", false
	}
	raw, ok := m["messages"]
	if !ok {
		return "", false
	}
	b, err := json.Marshal(raw)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return "", false
	}
	return string(b), true
}

// extractPreStepMessagesJSON 从插件返回的 result_json 里取 messages。
func extractPreStepMessagesJSON(resultJSON string) (string, bool) {
	if resultJSON == "" {
		return "", false
	}
	var payload struct {
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &payload); err != nil {
		return "", false
	}
	if len(payload.Messages) == 0 || string(payload.Messages) == "null" {
		return "", false
	}
	return string(payload.Messages), true
}

// wantsPreStepTransform 报告该插件是否声明了「可组合的请求面变换」能力。
func (m *Manager) wantsPreStepTransform(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info := m.coreMetadata[name]
	if info == nil {
		return false
	}
	v, ok := info.Capabilities[capPreStepTransform]
	return ok && v != "false"
}

// decodeEventPayload 把事件载荷还原成调用方期望的形态。
//
// 正常路径返回的是 json.Unmarshal 出来的 any（map[string]any）；提前返回若直接给回
// 原始 data（可能是 map[string]string 之类），调用方的类型断言会失配而静默当作「没有
// 改写」——所以这里统一成同一形态。
func decodeEventPayload(dataJSON string, fallback any) any {
	if dataJSON == "" {
		return fallback
	}
	var out any
	if err := json.Unmarshal([]byte(dataJSON), &out); err != nil {
		return fallback
	}
	return out
}

// reducedPreStepPayload 生成 pre-step 的「轻载荷」：同一个信封，但不带消息列表。
//
// 岗位之外的插件拿不到改写权，把几 MB 的消息复制过去没有任何用处。轻载荷保留
// session / user_input / 各计数——副作用需要的正是这些（重置重复调用链、记忆落盘
// 都只读这几个字段）。空串表示形状不认识，调用方退回完整载荷（宁可多传，不可把
// 不认识的信封改坏）。
func reducedPreStepPayload(data any) string {
	switch ev := data.(type) {
	case AgentPreStepEvent:
		ev.MessagesJSON = ""
		if b, err := json.Marshal(ev); err == nil {
			return string(b)
		}
	case map[string]any:
		trimmed := make(map[string]any, len(ev))
		for k, v := range ev {
			if k == "messages_json" {
				continue
			}
			trimmed[k] = v
		}
		if b, err := json.Marshal(trimmed); err == nil {
			return string(b)
		}
	}
	return ""
}

// 约定形状：{"append":[{"role":"system","content":"…"}]}；无该字段返回 nil。
func collectPreStepAppends(resultJSON string) []any {
	var payload struct {
		Append []any `json:"append"`
	}
	if err := json.Unmarshal([]byte(resultJSON), &payload); err != nil {
		return nil
	}
	return payload.Append
}

func (m *Manager) dispatchEventToPlugins(name EventName, data any) (any, error) {
	clients := m.hookClientsNamedSnapshot()
	if len(clients) == 0 {
		return data, nil
	}
	// pre-step 分两阶段（结构改写 → 请求面投影），载荷里带阶段标记，插件据此决定
	// 这一步该做哪件事。旧插件忽略该字段，行为与从前一致（它们只会在第一阶段生效）。
	payloadData := data
	if name == EventAgentPreStep {
		payloadData = withPreStepStage(data, PreStepStageCompaction)
	}
	dataJSON := ""
	if payloadData != nil {
		if b, err := json.Marshal(payloadData); err == nil {
			dataJSON = string(b)
		}
	}
	mode := eventDispatchMode[name]
	if mode == "" {
		mode = "emit"
	}

	// 压缩岗位：只有一个插件当值，它的改写被采纳、也只有它需要完整消息列表。
	// 岗位在启动期就确定（配置点名 + 已加载 + 声明 compaction），不是广播之后再挑。
	// 两个事件都走同一岗位：request-error 的重试决定权也一样——否则后端返回的
	// {"retry": true}（溢出紧急压缩后要求重试）会被丢弃，首次超窗就没有安全阀。
	role := CompactionRole{}
	if name == EventAgentPreStep || name == EventAgentRequestError {
		role = m.ResolveCompactionRole()
	}

	// request-error 只发给岗位持有者：只有它会做紧急压缩。广播给其他声明了 compaction
	// 的插件，它们会真的再压一遍（调模型、写状态），结果却被丢弃。岗位空缺时没人该动。
	if name == EventAgentRequestError {
		if role.Holder == "" {
			return decodeEventPayload(dataJSON, data), nil
		}
		kept := clients[:0:0]
		for _, nc := range clients {
			if nc.name == role.Holder {
				kept = append(kept, nc)
			}
		}
		if len(kept) == 0 {
			return decodeEventPayload(dataJSON, data), nil
		}
		clients = kept
	}

	// pre-step 的完整消息列表只发给岗位持有者，以及显式声明需要的观察者。其他人拿到
	// 同一信封但不带消息（副作用只需 session / user_input / 计数）。省掉的是每步
	// 几 MB 的复制 + 非岗位后端的无效摘要调用。
	lightJSON := ""
	fullPayload := map[string]bool{}
	if name == EventAgentPreStep {
		lightJSON = reducedPreStepPayload(payloadData)
		for _, nc := range clients {
			if m.wantsPreStepMessages(nc.name) {
				fullPayload[nc.name] = true
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventDispatchTimeout())
	defer cancel()

	switch mode {
	case "waterfall":
		// 洋葱模型：从尾向前包装，每个插件可改写 result_json
		// 初始 result 为原始 dataJSON；每个插件收到的 dataJSON 是上游改写后的
		var appends []any // 所有插件请求的一次性插播消息（与改写权无关）
		next := func() (string, error) { return dataJSON, nil }
		for i := len(clients) - 1; i >= 0; i-- {
			nc := clients[i]
			adopt := role.Holder != "" && nc.name == role.Holder
			innerNext := next
			next = func() (string, error) {
				payload := dataJSON
				if lightJSON != "" && nc.name != role.Holder && !fullPayload[nc.name] {
					payload = lightJSON
				}
				resp, err := nc.client.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: payload})
				if err != nil {
					// 传输层失败 ≠ veto：该插件只是没参与本轮（进程刚退出、
					// 二进制未升级导致收包上限偏小、超时等）。此前这里直接短路，
					// 于是一个**无关插件**的失败会把压缩后端的改写整条丢掉，
					// 宿主随即把未改写的全量历史发给 provider → 400
					// （prompt is too long）。记录并交给下游继续。
					m.logger.Warn("plugin event transport error, continuing chain",
						"event", string(name), "plugin", nc.name, "error", err.Error())
					return innerNext()
				}
				if resp.GetError() != "" {
					// 应用层显式报错仍是 veto：插件确实拒绝处理该事件。
					return "", fmt.Errorf("%s: %s", nc.name, resp.GetError())
				}
				if r := resp.GetResultJson(); r != "" {
					// 插播消息：任何插件都可请求（只影响消息尾部，不改写既有内容），
					// 与「载荷改写只归压缩后端」互不冲突。
					if name == EventAgentPreStep {
						appends = append(appends, collectPreStepAppends(r)...)
					}
					if adopt {
						dataJSON = r // 只有选定后端可改写载荷
					}
				}
				return innerNext()
			}
		}
		finalResult, err := next()
		if err != nil {
			return nil, err
		}
		// 反序列化最终 result 为 any（调用方自行类型断言）
		var result any
		if finalResult != "" {
			_ = json.Unmarshal([]byte(finalResult), &result)
		}
		// 第二阶段：请求面投影。在结构改写之后、对改写后的视图依次作用，与谁当值无关
		// ——投影不是「压缩的一部分」，压缩换人也该照跑。
		if name == EventAgentPreStep {
			result = m.applyPreStepProjections(ctx, clients, data, result)
		}
		if len(appends) > 0 {
			// 合并进返回载荷的 append 字段，交由调用方追加到消息末尾
			m, _ := result.(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			merged, _ := m["append"].([]any)
			m["append"] = append(merged, appends...)
			result = m
		}
		return result, nil

	case "serial":
		// 顺序调用，遇 error 即停
		for _, nc := range clients {
			resp, err := nc.client.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
			if err != nil {
				return nil, err
			}
			if resp.GetError() != "" {
				return nil, fmt.Errorf("%s", resp.GetError())
			}
		}
		return data, nil

	case "bail":
		// 首个非空 result 或 error 短路返回
		for _, nc := range clients {
			resp, err := nc.client.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
			if err != nil {
				return nil, err
			}
			if resp.GetError() != "" {
				return nil, fmt.Errorf("%s", resp.GetError())
			}
			if r := resp.GetResultJson(); r != "" {
				var result any
				_ = json.Unmarshal([]byte(r), &result)
				return result, nil
			}
		}
		return data, nil

	default: // "emit"
		// 同步顺序调用，忽略返回值（兼容现有通知型 listener）
		for _, nc := range clients {
			_, _ = nc.client.OnEvent(ctx, &proto.OnEventRequest{Name: string(name), DataJson: dataJSON})
		}
		return data, nil
	}
}
