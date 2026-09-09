package core

// 本文件定义「agent 回合生命周期」宿主事件（对齐 DSH 的 agent/status 与
// agent/error 原生事件）：主 agent 比回合生命周期时点广播通用事件——成功
// （success 终帧）发 agent/status(idle)，失败（error 终帧）发 agent/error；
// 回合开始发 agent/status(running)；进入 LLM 请求前发 agent/pre-step；
// LLM 请求失败发 agent/request-error。事件经 EventBus 同步分发（dispatchEventToPlugins）
// 给订阅插件（Hook.OnEvent），是任何插件皆可独立订阅的系统级能力（如通知插件
// 区分成败音效、billion-context 插件拦截 pre-step 改写消息列表）。
//
// 分发模式对齐 DSH Cordis ctx.on()：模式由事件名决定（见 eventDispatchMode
// 映射表），listener 不感知模式——通知型 listener 忽略 next、返回 ("", nil)；
// 拦截型 listener 调 next 委托下游、返回改写后的 result_json。

const (
        // EventAgentStatus agent 回合状态事件（对齐 DSH agent/status）。
        // 宿主主 agent 每回合（RunStream）广播一次：
        //   running — 回合开始；
        //   idle    — 回合成功完成（success 终帧后）。
        EventAgentStatus EventName = "agent/status"

        // EventAgentError agent 回合报错事件（对齐 DSH agent/error）。
        // 回合以 error 终帧失败收尾时广播，供插件区分成功/失败（如通知音效）。
        EventAgentError EventName = "agent/error"

        // EventAgentPreStep 进入 LLM 请求前事件（对齐 DSH agent/pre-step，waterfall 模式）。
        // agent 在收集本步要发给 LLM 的消息列表之后、实际发送请求之前广播。
        // 拦截型 listener 可改写消息列表（如 billion-context 注入 <acp> 标签、应用 prune、
        // 注入 nudge）；通知型 listener 可记录每步开始时点。
        // 载荷：AgentPreStepEvent{Agent, Turn, Step, MessagesJSON, Signal}
        EventAgentPreStep EventName = "agent/pre-step"

        // EventAgentRequestError LLM 请求失败事件（对齐 DSH agent/request-error，waterfall 模式）。
        // LLM 调用返回非 nil 错误时广播。拦截型 listener 可决定是否重试——返回非空
        // ResultJSON 含 {"retry": true} 时 agent 重新发起请求（如 billion-context 触发
        // 上下文溢出压缩后重试）；不调 next 或返回空 result 视为放弃重试。
        // 载荷：AgentRequestErrorEvent{Agent, Turn, Step, Error, Code}
        EventAgentRequestError EventName = "agent/request-error"
)

// AgentStatusValue agent/status 事件的状态取值（对齐 DSH AgentStatus）。
type AgentStatusValue string

const (
        // AgentStatusRunning 回合进行中。
        AgentStatusRunning AgentStatusValue = "running"
        // AgentStatusIdle 回合完成，agent 回到空闲。
        AgentStatusIdle AgentStatusValue = "idle"
)

// AgentStatusEvent agent/status 事件的载荷（对齐 DSH { agent, status }，
// 此处 agent 名用字符串，status 为运行态取值）。
type AgentStatusEvent struct {
        Agent  string           `json:"agent"`  // agent 插件名
        Status AgentStatusValue `json:"status"` // running | idle
}

// AgentErrorEvent agent/error 事件的载荷（对齐 DSH { agent, turn, step, error }）。
type AgentErrorEvent struct {
        Agent string `json:"agent"` // agent 插件名
        Error string `json:"error"` // 失败原因文本
}

// AgentPreStepEvent agent/pre-step 事件的载荷（对齐 DSH agent/pre-step）。
// MessagesJSON 为本步要发给 LLM 的消息列表 JSON（含 system + 历史 + 本步 user）。
// listener 在 waterfall 模式下可改写此字段并经 result_json 返回改写后的列表。
type AgentPreStepEvent struct {
        Agent        string `json:"agent"`         // agent 插件名
        Turn         int    `json:"turn"`          // 当前回合号（1-based）
        Step         int    `json:"step"`          // 当前步号（1-based，回合内递增）
        MessagesJSON string `json:"messages_json"` // 本步消息列表 JSON（proto.Message 数组序列化）
        TokenCount   int    `json:"token_count"`   // 估算的当前上下文 token 数（供 nudge 决策）
}

// AgentRequestErrorEvent agent/request-error 事件的载荷（对齐 DSH agent/request-error）。
// listener 在 waterfall 模式下可返回非空 result_json 含 {"retry": true} 触发重试。
// Code 取值：context_window_exceeded / rate_limited / network_error / unknown。
type AgentRequestErrorEvent struct {
        Agent string `json:"agent"` // agent 插件名
        Turn  int    `json:"turn"`  // 回合号
        Step  int    `json:"step"`  // 步号
        Error string `json:"error"` // 错误文本
        Code  string `json:"code"`  // 稳定错误码（如 context_window_exceeded）
}

// eventDispatchMode 事件名 → 分发模式映射（对齐 DSH Cordis：模式由事件名决定，
// listener 不感知模式）。未列事件默认 "emit"（向后兼容现有通知型 listener）。
//
// 设计动机：DSH 的 ctx.on() 让所有事件用同一注册接口，分发模式由发射端决定
// （emit/serial/bail/waterfall）。本表把这种「模式是事件属性」的语义落地：
// 新增事件只需在此表加一行声明分发模式，listener 端无需任何改动。
var eventDispatchMode = map[EventName]string{
        EventAgentPreStep:      "waterfall", // 可改写消息列表
        EventAgentRequestError: "waterfall", // 可决定是否重试
        // 其余事件默认 "emit"（纯通知，忽略返回值）：
        //   agent/status, agent/error, approval/asked, approval/decided,
        //   approval/policy, tools/result, tools/change, job/done, llm/request,
        //   tools/pre-execute, tools/execute, tools/post-execute
}
