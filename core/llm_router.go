package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"dsc/proto"
	plugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// 多 provider 路由（对齐 DSH route）：agent 只连接单个「聚合 LLM 服务」，
// 服务按 primary（agent 声明的 provider）→ fallback（其余已加载 provider，
// 按加载顺序）依次尝试；每次尝试都经过 llm/request 瀑布（含内建退避重试），
// 前一个 provider 失败且未产生输出时切到下一个。

// llmAggregateServer 聚合 LLM 服务：动态读取 Manager 中已加载的 provider。
type llmAggregateServer struct {
	proto.UnimplementedLLMServiceServer
	m *Manager
}

// Chat 依次尝试 provider（primary 优先），首个成功即返回；全部失败返回最后的错误。
// 路由顺序与 provider 在 llmRouteSnapshot 的 RLock 下打成快照，调用期间不持锁
// （避免长调用阻塞热重载，同时消除与热重载写 m.llms 的并发 map 竞态）。
//
// 在调 provider 前先经 dispatchEventToPlugins 分发 agent/pre-step 事件（waterfall
// 模式）：声明了该能力的插件可改写消息列表——注入上下文标签、应用裁剪、
// 注入 nudge 等。改写后的消息列表透传给实际 provider。无插件监听时事件直接通过，
// 行为与原先一致（零开销）。
//
// 失败时经 agent/request-error 事件让插件决定是否重试：插件返回 {"retry": true}
// 时重新走一轮 applyPreStepHook + provider 调用（如插件触发溢出压缩后要求重试）。
// 最多重试 maxRetries 次防止无限循环。
func (s *llmAggregateServer) Chat(ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var err error
		if req, err = s.applyPreStepHook(ctx, req); err != nil {
			return nil, err
		}
		req = sanitizeRequestPairing(s.m, req)
		var lastErr error
		for _, np := range s.m.llmRouteSnapshot() {
			start := time.Now()
			call := &LLMCall{Provider: np.name, Request: req}
			err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
				resp, err := chatWithProvider(np.p, ctx, req)
				call.Response, call.Err = resp, err
				return err
			})
			if err == nil {
				// 逐次尝试结算留痕（-log 启用时的全链路诊断能力）：
				// provider/尝试序号/时长/finish_reason/产出规模一眼可查
				s.m.logger.Info("llm request settled",
					"provider", np.name, "streaming", false, "attempt", attempt+1,
					"duration_ms", time.Since(start).Milliseconds(),
					"finish_reason", call.Response.FinishReason,
					"content_chars", len(call.Response.Content),
					"tool_calls", len(call.Response.ToolCalls))
				return call.Response, nil
			}
			failErr := err
			if call.Err != nil {
				failErr = call.Err
			}
			s.m.logger.Warn("llm request failed",
				"provider", np.name, "streaming", false, "attempt", attempt+1,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", failErr.Error(), "code", ClassifyLLMErrorCode(failErr))
			if call.Err != nil {
				lastErr = call.Err
			} else {
				lastErr = err
			}
		}
		// 所有 provider 失败：检查插件是否要求重试
		if attempt < maxRetries {
			if shouldRetry := s.emitRequestError(ctx, req, lastErr); shouldRetry {
				s.m.logger.Info("agent/request-error: plugin requested retry",
					"attempt", attempt+1, "error", lastErr.Error())
				continue
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no LLM provider available")
		}
		return nil, lastErr
	}
	return nil, fmt.Errorf("all providers failed after %d retries", maxRetries)
}

// ChatStream 依次尝试 provider：前一个 provider 在未产生任何帧时失败才切下一个
// （已发帧后失败不切换，避免重复输出）。
//
// 同 Chat：调 provider 前先经 agent/pre-step 事件让插件改写消息列表。
// 失败时经 agent/request-error 事件让插件决定是否重试（最多 maxRetries 次）。
func (s *llmAggregateServer) ChatStream(req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer) error {
	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		var preErr error
		if req, preErr = s.applyPreStepHook(stream.Context(), req); preErr != nil {
			return preErr
		}
		req = sanitizeRequestPairing(s.m, req)
		var lastErr error
		providerTried := false
		for _, np := range s.m.llmRouteSnapshot() {
			start := time.Now()
			call := &LLMCall{Provider: np.name, Request: req}
			err := s.m.events.Waterfall(EventLLMRequest, EventContext{Data: call}, func(EventContext) error {
				return chatStreamWithProvider(np.p, req, stream, call)
			})
			if err == nil {
				s.m.logger.Info("llm request settled",
					"provider", np.name, "streaming", true, "attempt", attempt+1,
					"duration_ms", time.Since(start).Milliseconds(),
					"finish_reason", call.FinishReason,
					"tool_calls", call.ToolCalls,
					"usage", streamUsageSummary(call.Usage))
				// 按会话记录最近一次成功请求的上报 prompt 用量：agent/pre-step 的压缩
				// 压力判定以此精确底数 + 启发式增量（对齐 DSH tokenMeter pressureTokens）。
				// 仅流式请求携带 usage；非流式 Chat 不记录（保持 0 → 插件退回启发式）。
				if call.Usage != nil && call.Usage.PromptTokens > 0 {
					// 量一次**最终发出表面**（消息 + 工具目录）：估算与构成出自同一遍扫描，
					// 判超窗与喂校准表必须同源。
					stats := MeasureRequestSurface(req.Messages, req.Tools)
					// 记录锚点：req 是经 pre-step 改写后的最终请求，所以这里的估算与
					// 指纹描述的就是真正发出的那个表面（压缩后端改写后的）。
					s.m.recordSessionAnchor(req.GetSessionId(), sessionUsageAnchor{
						promptTokens:  int(call.Usage.PromptTokens),
						sentEstimate:  stats.Estimate,
						toolsHash:     ToolsFingerprint(req.Tools),
						model:         s.m.modelName,
						contextWindow: s.m.contextWindow,
					})
					// 喂校准：账本（可审计/可重拟合）+ 按模型的校准表（跨会话、跨重启）。
					s.m.recordUsageSample(req.GetSessionId(), UsageSample{
						Model:   s.m.modelName,
						Sent:    stats.Estimate,
						Actual:  int(call.Usage.PromptTokens),
						Msgs:    stats.Msgs,
						MsgEst:  stats.MsgEstimate,
						ToolEst: stats.ToolEstimate,
						Classes: stats.Classes,
					})
				}
				return nil
			}
			providerTried = true
			failErr := err
			if call.Err != nil {
				failErr = call.Err
			}
			s.m.logger.Warn("llm request failed",
				"provider", np.name, "streaming", true, "attempt", attempt+1,
				"duration_ms", time.Since(start).Milliseconds(),
				"stream_started", call.StreamStarted,
				"error", failErr.Error(), "code", ClassifyLLMErrorCode(failErr))
			if call.StreamStarted {
				// 已产生输出：不切换 provider，不重试，直接返回
				if call.Err != nil {
					return call.Err
				}
				return err
			}
			if call.Err != nil {
				lastErr = call.Err
			} else {
				lastErr = err
			}
		}
		// 所有 provider 失败（或无 provider）：检查插件是否要求重试
		if providerTried && attempt < maxRetries {
			if shouldRetry := s.emitRequestError(stream.Context(), req, lastErr); shouldRetry {
				s.m.logger.Info("agent/request-error: plugin requested retry",
					"attempt", attempt+1, "error", lastErr.Error())
				continue
			}
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("no LLM provider available")
		}
		return lastErr
	}
	return fmt.Errorf("all providers failed after %d retries", maxRetries)
}

// sanitizeRequestPairing 在把请求交给 provider 前整理 tool_call 与 tool_result 的配对。
//
// 放在 pre-step 之后：压缩插件会改写消息（可能丢掉某些结果），整理要看的是最终将发出去
// 的那一份。整理本身幂等，重试再次调用不会累积改动。
//
// 丢弃不是无声的：provider 侧的报错（"tool calls and tool results do not match"）发生在
// 请求已经发出之后，用户只能「新建任务」；这里丢掉并在日志留一条，至少能看出是哪一轮
// 少了什么。
func sanitizeRequestPairing(m *Manager, req *proto.ChatRequest) *proto.ChatRequest {
	if req == nil || len(req.GetMessages()) == 0 {
		return req
	}
	fixed, dropped := SanitizeToolPairing(req.GetMessages())
	// 无论有没有丢弃都要回写：插话落在调用与结果之间时，一条都不用丢，
	// 但顺序必须换（结果紧跟调用）。只在丢东西时才记日志。
	req.Messages = fixed
	if dropped > 0 {
		m.logger.Warn("llm request: repaired tool call/result pairing",
			"dropped", dropped, "messages", len(req.GetMessages()), "kept", len(fixed),
			"hint", "工具调用与其结果不再一一对应（插话落在调用与结果之间、或工具被中断）；已丢弃无法配对的部分")
	}
	return req
}

// applyPreStepHook 在 LLM 请求前分发 agent/pre-step 事件（waterfall 模式），
// 让订阅了 pre-step 的插件有机会改写消息列表。无插件监听或返回空 result
// 时原样返回 req（零开销）。
//
// 改写协议：插件返回的 result_json 应为 {"messages": [...]} 形式，其中 messages
// 为改写后的 proto.Message 数组。宿主解析后替换 req.Messages。
func (s *llmAggregateServer) applyPreStepHook(ctx context.Context, req *proto.ChatRequest) (*proto.ChatRequest, error) {
	// 先清掉上一发的「实际表面」：它只描述那一次请求，任何一条提前返回的路径
	// （无插件、分发出错）都必须把它作废，否则一份过时的数字会一直压着界面，
	// 让「已用」冻在一个不存在的请求上。
	s.m.NoteActualSurface(req.GetSessionId(), 0)

	msgs, changed := req.Messages, false
	if len(s.m.hookClientsSnapshot()) > 0 {
		msgs, changed = s.rewriteViaPlugins(ctx, req)
	}
	return s.finishPreStep(req, msgs, changed)
}

// rewriteViaPlugins 分发 agent/pre-step，让订阅的插件改写消息列表；返回最终消息与
// 「是否真的变了」。分发出错时按未改写返回（并记 ERROR，便于把 provider 的 4xx 与
// 这里的失败对上）。
func (s *llmAggregateServer) rewriteViaPlugins(ctx context.Context, req *proto.ChatRequest) ([]*proto.Message, bool) {
	msgsJSON, _ := json.Marshal(req.Messages)
	// 估算当前 token 数（复用 core 公用启发式 EstimateProtoMessagesTokens，
	// CJK 感知 + 角色开销 + 图像 + 工具参数，供插件 nudge 决策）——
	// 与压缩插件同一估算口径，避免各处字节/4 粗略估算漂移。
	// 必须把工具目录一起算：它每次请求都随载荷发出，只算消息会系统性低估压力。
	tokenCount := EstimateProtoMessagesTokens(req.Messages) + EstimateProtoToolTokens(req.Tools)
	// 投影压力：由宿主统一给出（消息+工具的表面大小，信封可比时以 provider 用量为底数）。
	// 压缩后端不再各自合成这个数——决策输入只应有这一处来源（对齐 DSH：消费者读
	// tokenMeter，不自己算）。
	anchor, _ := s.m.lastSessionAnchor(req.GetSessionId())
	projected, matched := projectPressure(anchor, tokenCount, ToolsFingerprint(req.Tools), s.m.modelName, s.m.contextWindow)
	result, err := s.m.dispatchEventToPlugins(EventAgentPreStep, AgentPreStepEvent{
		Agent:           s.m.GetMainAgentName(),
		Session:         req.GetSessionId(),
		MessagesJSON:    string(msgsJSON),
		TokenCount:      tokenCount,
		ToolTokens:      EstimateProtoToolTokens(req.Tools),
		LastUsageTokens: anchor.promptTokens,
		ProjectedTokens: projected,
		EnvelopeMatched: matched,
		UserInput:       req.GetNewUserInput(),
	})
	if err != nil {
		// 回退语义是「按未改写的原始请求发出」——对长会话等于把全量历史直接怼给
		// provider（压缩后端白跑）。所以记 ERROR 而不是 WARN，并带上规模，
		// 便于把 provider 的 4xx 与这里的失败对上（gRPC 4 MiB 收包上限曾是元凶）。
		s.m.logger.Error("agent/pre-step hook failed; sending UNREWRITTEN request",
			"error", err.Error(),
			"session", req.GetSessionId(),
			"messages", len(req.Messages),
			"messages_bytes", len(msgsJSON),
			"estimated_tokens", tokenCount)
		return req.Messages, false
	}
	if result == nil {
		return req.Messages, false
	}
	resultMap, ok := result.(map[string]any)
	if !ok {
		return req.Messages, false
	}
	msgs := req.Messages
	changed := false
	if msgsRaw, ok := resultMap["messages"]; ok {
		newMsgsJSON, _ := json.Marshal(msgsRaw)
		var newMsgs []*proto.Message
		if err := json.Unmarshal(newMsgsJSON, &newMsgs); err == nil && len(newMsgs) > 0 {
			msgs = newMsgs
			changed = true
		}
	}
	// 一次性插播消息：插件（如记忆）在任务收尾时请求插入一条 system 角色的提示。
	// 走「追加到消息末尾」而不是改 system prompt——系统提示是缓存前缀，任何改动都会
	// 让整段前缀缓存失效；插播消息只影响尾部，代价小得多。
	if ups := parsePreStepAppends(resultMap["append"]); len(ups) > 0 {
		msgs = append(append([]*proto.Message{}, msgs...), ups...)
		changed = true
	}
	return msgs, changed
}

// finishPreStep 收口：量最终表面、记录展示口径、执行窗口硬上限，返回要发出的请求。
//
// 硬上限（enforceRequestCeiling）是最后一道确定性防线：压缩后端把 1.66M 渲染成 1.07M
// 就发出去过一次，provider 用 400 告诉我们 1,048,576 才是天花板。改写后的视图有多大都
// 由后端自己决定，它并没有「必须塞进窗口」的义务，所以这里必须兜住——宁可从头丢掉
// 最旧的一段，也不发一个必然被拒的请求。裁剪安全的前提是消息身份为宿主下发的稳定 id
// （event-<seq>），裁掉头部不会让压缩块的引用错位。
//
// 为什么超窗时直接滑窗，而**不**先请压缩后端紧急压缩一次：
//
//   - 紧急路径产出的是**没有内容的占位摘要**——一类后端写「N 条消息已压缩」的固定模板，
//     另一类强制走截断摘要（`emergency=true` 直接跳过 LLM）。一次紧急压缩就把几百条
//     真实消息永久埋进一句空话，而滑窗只影响这一次请求（会话日志一字不动，下次仍可
//     正常压缩）；
//   - 一次滑窗必然装得下，而「压一个范围再重试」不保证收敛：1.07M 那次连续三次重试都
//     没压进窗口，最后仍是整轮失败；
//   - 真正需要紧急压缩的信号是 provider 的 4xx——那说明我们量错了（估算偏差），而不是
//     预测性地提前挤压历史。那条路仍然保留（provider 4xx → agent/request-error → 后端
//     紧急压缩 → 重试）。
//
// 所以顺序是：改写 → 量 → 装不下就滑窗；后端该做的是它自己阈值内的正常压缩，不是替
// 宿主擦超窗的屁股。
func (s *llmAggregateServer) finishPreStep(req *proto.ChatRequest, msgs []*proto.Message, changed bool) (*proto.ChatRequest, error) {
	toolsTokens := EstimateProtoToolTokens(req.Tools)
	session := req.GetSessionId()

	surface := EstimateProtoMessagesTokens(msgs) + toolsTokens
	trimmed, dropped := msgs, 0
	// 窗口取「配置值」与「provider 亲口报过的上限」的较小者：被 4xx 教过一次之后，
	// 就不必每次都去撞那面墙。
	window := s.m.effectiveWindow(session)
	// 窗口未知（0）时**不能跳过收口**：那等于把整份派生历史原样发给 provider，
	// 而历史往往远超任何窗口——实测一次 98MB 会话重启后就这样发了 2,060,734 的估算，
	// provider 直接 400（1,268,641 > 1,048,576）。窗口没装配好是宿主自己的问题，
	// 不该由「发一个必然被拒的请求」来暴露；这里按 128K 保守兜底并告警。
	if window <= 0 {
		window = defaultCeilingWindow
		s.m.logger.Warn("context window unknown at request time; enforcing the conservative default",
			"session", session, "window", window,
			"hint", "宿主未拿到窗口（配置/探测都缺）：按默认值收口，避免发出必然被 provider 拒绝的请求")
	}
	if window > 0 {
		inflation := s.m.ceilingInflation(session, s.m.modelName)
		// 收口比例：有本会话实测才用 0.90 的常规余量；续长会话的首帧没有依据，
		// 用更保守的 0.80；被 provider 拒过之后逐次递减，保证重试一定收敛。
		ratio := contextCeilingRatio
		if _, ok := s.m.lastSessionAnchor(session); !ok {
			ratio = contextCeilingUnvalidatedRatio
		}
		if adj, ok := s.m.overflowFor(session); ok {
			if r := overflowTrimRatio(adj.attempts); r > 0 && r < ratio {
				ratio = r
			}
		}
		var before, after int
		var fits bool
		trimmed, dropped, before, after, fits = enforceRequestCeilingAt(msgs, toolsTokens, window, inflation, ratio)
		surface = after
		if !fits {
			// 两种失败要分开说，否则用户没法判断该怎么做：
			//   - 真超窗：provider 一定会拒，只能缩输入/换模型；
			//   - 没超窗但装不进收口余量：这是本地的保守预算（窗口×比例），
			//     用户看到的数字比窗口小，却收到「超出上下文窗口」——现场就是这么
			//     误导的：236,004 < 262,144 被判超窗，报错还写着窗口 262,144。
			limit := ceilingLimit(window, inflation, ratio)
			if after > window {
				return nil, fmt.Errorf("本轮内容超出上下文窗口：滑窗后仍有 %d tokens（窗口 %d，偏差系数 %.2f）。"+
					"更早的消息已全部省略，剩下的是这一轮本身——请缩短本次输入（如大文件/长输出）或换更大窗口的模型",
					after, window, inflation)
			}
			return nil, fmt.Errorf("本轮内容塞不进收口余量：滑窗后仍有 %d tokens"+
				"（收口上限 %d = 窗口 %d × %.0f%%，偏差系数 %.2f）——内容本身没有超窗，"+
				"但首条 system（部署指令/技能/记忆）与本轮这一段就占满了余量，已无可丢的历史。"+
				"请缩短本次输入（如大文件/长输出）、换更大窗口的模型，或精简 system 装配",
				after, limit, window, ratio*100, inflation)
		}
		if dropped > 0 {
			s.m.logger.Warn("llm request: trimmed to fit the context window",
				"session", session, "dropped", dropped,
				"messages", len(msgs), "kept", len(trimmed),
				"estimated_before", before, "estimated_after", after,
				"window", window, "inflation", inflation,
				"hint", "改写后的视图仍超窗口：已按滑动窗口丢弃最旧的一段（会话日志不动）；压缩后端应在自己阈值内正常压缩，而不是靠紧急压缩擦超窗")
		}
	}

	// 记录本步**实际发出**的表面（消息 + 工具目录）。压缩后端改写请求之后
	// （900k → 摘要 + 尾部），宿主与 agent 看到的仍是未改写的全量历史：展示若照用
	// 那个数，界面会一直显示 90% 而压缩插件按 40% 判断「还没到阈值」，看起来就是
	// 「到了阈值却不动」。展示口径必须跟真正发出去的东西走。
	if changed || dropped > 0 {
		s.m.NoteActualSurface(session, surface)
	}
	// 输出预算（max_tokens）在这里算定，而不是让每个 provider 插件各自按窗口估一遍：
	// 只有宿主持有「窗口」「本帧最终表面（含工具目录）」「服务端认过的上限」这三样。
	// 调用方已经给了值（压缩等场景的净余值）就不动——那是它按自己的语义算过的。
	budget := 0
	if req.GetMaxTokens() == 0 {
		budget = s.m.outputBudgetFor(surface, window)
	}
	if !changed && dropped == 0 && budget == 0 {
		return req, nil
	}
	// 返回改写后的请求（不修改原 req，避免影响重试路径的原始数据）。
	// 会话归属与新用户输入标记随写复制：重试路径再次分发 pre-step 时语义不变。
	out := &proto.ChatRequest{
		Messages:     trimmed,
		Tools:        req.Tools,
		MaxTokens:    req.MaxTokens,
		SessionId:    req.SessionId,
		NewUserInput: req.NewUserInput,
	}
	if budget > 0 {
		out.MaxTokens = int32(budget)
	}
	return out, nil
}

// parsePreStepAppends 解析插件经 pre-step 返回的一次性插播消息：
// {"append":[{"role":"system","content":"…"}]}。role 仅接受 system/user
// （assistant/tool 需要对应的调用配对，插件无从构造，遇到即丢弃）。
func parsePreStepAppends(raw any) []*proto.Message {
	if raw == nil {
		return nil
	}
	norm, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var items []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(norm, &items); err != nil {
		return nil
	}
	out := make([]*proto.Message, 0, len(items))
	for _, it := range items {
		content := strings.TrimSpace(it.Content)
		if content == "" {
			continue
		}
		role := it.Role
		if role == "" {
			role = "system"
		}
		if role != "system" && role != "user" {
			continue
		}
		out = append(out, &proto.Message{Role: role, Content: content})
	}
	return out
}

// sessionUsageAnchor 宿主侧 per-session 用量锚点（供 pre-step 计算投影压力）。
type sessionUsageAnchor struct {
	promptTokens  int    // provider 上报的 prompt 侧用量（实测）
	sentEstimate  int    // 产生该用量时，对同一请求（消息+工具）的启发式估算
	toolsHash     string // 产生该用量时的工具目录指纹（空 = 未知/不可比）
	model         string // 产生该用量时的模型名（空 = 未知）
	contextWindow int    // 产生该用量时的上下文窗口（0 = 未知）
}

// sessionAnchorSet 一个会话的用量锚点：最近一次（与信封无关，供热路径读）加上
// 按模型分开的一份。
//
// 为什么要按模型分开：估算偏差（provider 计数 / 我们的估算）是分詞器相关的属性。
// 换模型后比例会变，但**不该因此退回 1**——那等于在最缺依据的时刻（刚切过去、还没
// 产生任何新用量）放弃全部记忆。所以这里保留每个模型最近一次的比例：当前模型有记录
// 就用它，没有就借「上一个用过的那份」，等这一帧的新用量结算后自动换成自己的。
type sessionAnchorSet struct {
	latest  sessionUsageAnchor
	byModel map[string]sessionUsageAnchor
}

// recordSessionAnchor 按会话记录最近一次成功 LLM 请求的用量锚点
// （对齐 DSH tokenMeter 的 pressureTokens + request header 语义）。空会话 id 或
// 非正用量忽略（内部调用/非流式请求无上报值，不污染会话状态）。
func (m *Manager) recordSessionAnchor(sessionID string, a sessionUsageAnchor) {
	if sessionID == "" || a.promptTokens <= 0 {
		return
	}
	// 成功的请求证明「当前预算可用」：清掉超窗时学到的临时收紧（一次异常不该让这个
	// 会话永远按放大系数裁）。系统性的偏差由这个锚点继续持有（estimateInflation）。
	m.clearOverflow(sessionID)
	m.sessionAnchorMu.Lock()
	defer m.sessionAnchorMu.Unlock()
	set := m.sessionAnchors[sessionID]
	if set == nil {
		set = &sessionAnchorSet{byModel: make(map[string]sessionUsageAnchor, 2)}
		m.sessionAnchors[sessionID] = set
	}
	set.latest = a
	if a.model != "" {
		// 只留最近几个：一个会话切模型的次数很少，但别让这张表无限长。
		if len(set.byModel) >= 4 {
			for k := range set.byModel {
				if k != a.model {
					delete(set.byModel, k)
					break
				}
			}
		}
		set.byModel[a.model] = a
	}
}

// lastSessionAnchor 返回某会话最近一次的用量锚点（ok=false = 无记录：
// 首次请求、重启恢复或仅走非流式 Chat）。
func (m *Manager) lastSessionAnchor(sessionID string) (sessionUsageAnchor, bool) {
	if m == nil || sessionID == "" {
		return sessionUsageAnchor{}, false
	}
	m.sessionAnchorMu.RLock()
	defer m.sessionAnchorMu.RUnlock()
	set := m.sessionAnchors[sessionID]
	if set == nil {
		return sessionUsageAnchor{}, false
	}
	return set.latest, true
}

// anchorForModel 取「用来推断估算偏差」的锚点：当前模型自己的优先，没有就借最近一次
// 别的模型的（borrowed=true）。
//
// 借用是显式的、临时的：它只影响下一帧的预算判定，这一帧结算出的用量立刻写进
// 当前模型名下，下一帧就用它自己的比例。宁可先用别家的数（偏差通常只差一两成），
// 也不要在刚切模型时把系数退回 1——那是我们已知最危险的一侧。
func (m *Manager) anchorForModel(sessionID, model string) (sessionUsageAnchor, bool, bool) {
	if m == nil || sessionID == "" {
		return sessionUsageAnchor{}, false, false
	}
	m.sessionAnchorMu.RLock()
	defer m.sessionAnchorMu.RUnlock()
	set := m.sessionAnchors[sessionID]
	if set == nil {
		return sessionUsageAnchor{}, false, false
	}
	if model != "" {
		if a, ok := set.byModel[model]; ok {
			return a, true, false
		}
	}
	if set.latest.promptTokens > 0 && set.latest.sentEstimate > 0 {
		return set.latest, true, true
	}
	return sessionUsageAnchor{}, false, false
}

// projectPressure 由「锚点 + 当前表面」得出本轮压力（对齐 DSH tokenMeter.measure 的
// baseline + surfaceDelta）：
//
//  1. 信封不可比（工具目录/窗口不同，或旧记录无指纹）→ 直接用当前启发式估算；
//  2. 信封可比，但 provider 用量低于该信封的启发式价（说明那次请求被压缩/改写，
//     用量描述的不是这个表面）→ 同样退回估算；
//  3. 否则以 provider 用量为底数，只加「自采样以来的表面差量」（可正可负，下限 0）。
//
// matched 表示信封是否可比（供插件日志与调试）。
func projectPressure(anchor sessionUsageAnchor, curEstimate int, curToolsHash, curModel string, window int) (projected int, matched bool) {
	if curEstimate < 0 {
		curEstimate = 0
	}
	matched = envelopeMatches(anchor, curToolsHash, curModel, window)
	if !matched || anchor.promptTokens < anchor.sentEstimate {
		return curEstimate, matched
	}
	p := anchor.promptTokens + (curEstimate - anchor.sentEstimate)
	if p < 0 {
		p = 0
	}
	return p, true
}

// emitRequestError 在 LLM 请求失败后分发 agent/request-error 事件（waterfall 模式），
// 让订阅了 pre-step 的插件有机会决定是否重试。返回 true 表示插件要求重试——
// 调用方（Chat/ChatStream）据此重新走一轮 applyPreStepHook + provider 调用。
//
// 插件经 result_json 返回 {"retry": true} 时视为重试请求。
// 典型场景：上下文溢出时压缩后端插件触发紧急压缩后要求重试。
func (s *llmAggregateServer) emitRequestError(ctx context.Context, req *proto.ChatRequest, err error) bool {
	if err == nil {
		return false
	}
	code := "unknown"
	if isContextWindowExceeded(err) {
		code = "context_window_exceeded"
	}
	// 超窗的处置权在宿主，不派给插件：那条路只能产出无内容的占位摘要（固定模板 /
	// 强制截断），一次就把几百条真实消息永久埋掉。这里改为「记住 provider 的实测值与
	// 上限 → 收紧本会话预算 → 要求重试」，重试时 pre-step 收口会滑窗（会话日志不动）。
	if code == "context_window_exceeded" {
		stats := MeasureRequestSurface(req.GetMessages(), req.GetTools())
		actual, max := overflowParse(err.Error())
		s.m.noteOverflow(req.GetSessionId(), actual, max, stats.Estimate)
		// 被拒的一次同样是真实证据：provider 亲口报了多少，除以我们算的多少，就是这一
		// 档内容规模的偏差。**记进账本与比例校准**，这样下一发（乃至下次启动）就有依据，
		// 而不是每次都在同一个规模上重新学。没有构成计数，所以标 NoClasses 让它只进
		// 比例、不进系数拟合。
		if actual > 0 {
			s.m.recordUsageSample(req.GetSessionId(), UsageSample{
				Model: s.m.modelName, Sent: stats.Estimate, Actual: actual,
				Msgs: stats.Msgs, MsgEst: stats.MsgEstimate, ToolEst: stats.ToolEstimate,
				NoClasses: true,
			})
		}
		return true
	}
	result, _ := s.m.dispatchEventToPlugins(EventAgentRequestError, AgentRequestErrorEvent{
		Agent: s.m.GetMainAgentName(),
		Error: err.Error(),
		Code:  code,
	})
	// 检查插件是否返回 {"retry": true}
	if result == nil {
		return false
	}
	if m, ok := result.(map[string]any); ok {
		if retry, ok := m["retry"]; ok {
			if b, ok := retry.(bool); ok && b {
				return true
			}
		}
	}
	return false
}

// isContextWindowExceeded 报告错误是否为上下文窗口溢出（粗略匹配常见 provider 错误文本）。
func isContextWindowExceeded(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "context length", "context window", "maximum context", "too long")
}

// ClassifyLLMErrorCode 把 LLM 调用错误归类为稳定错误码（对齐 DSH request-error
// 的稳定词汇）：context_window_exceeded / rate_limited / network_error / unknown；
// err 为 nil 返回空串。仅供日志与诊断留痕（llm/attempt 会话事件、宿主运行日志）——
// 重试协议（agent/request-error）不受此分类影响，仍按 isContextWindowExceeded 判定，
// 保证既有压缩插件的重试决策路径零变化。
func ClassifyLLMErrorCode(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "context length", "context window", "maximum context", "too long"):
		return "context_window_exceeded"
	case containsAny(msg, "rate limit", "rate_limit", "429", "too many requests", "quota"):
		return "rate_limited"
	case containsAny(msg, "connection", "timeout", "eof", "network", "dns", "refused", "reset by peer", "unavailable", "broken pipe"):
		return "network_error"
	default:
		return "unknown"
	}
}

// streamUsageSummary 把流式用量摘要为紧凑日志字符串（无用量时返回 "-"）。
func streamUsageSummary(u *Usage) string {
	if u == nil {
		return "-"
	}
	return fmt.Sprintf("prompt=%d completion=%d total=%d", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}

// containsAny 报告 s 是否包含 any 子串。
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// namedLLMProvider 路由快照中的单个 provider。
type namedLLMProvider struct {
	name string
	p    LLMProvider
}

// llmRouteSnapshot 在 RLock 下取路由顺序与 provider 快照后立即释放锁：
// 返回 [primary（若已加载）+ 其余按加载顺序] 的有序列表。供 Chat/ChatStream 在
// 调用期间不持锁（避免长调用阻塞热重载），同時消除 handler 侧无锁读 m.llms 与
// 热重载写 m.llms 的并发 map 竞态）。
func (m *Manager) llmRouteSnapshot() []namedLLMProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	order := make([]namedLLMProvider, 0, len(m.llmOrder)+1)
	if p, ok := m.llms[m.agentLLMName]; ok {
		order = append(order, namedLLMProvider{name: m.agentLLMName, p: p})
	}
	for _, n := range m.llmOrder {
		if n == m.agentLLMName {
			continue
		}
		if p, ok := m.llms[n]; ok {
			order = append(order, namedLLMProvider{name: n, p: p})
		}
	}
	return order
}

// serveAggregateLLMLocked 在 broker 上挂载「聚合 LLM 服务」并返回其 serviceID
// （需已持有 m.mu）。已挂载则复用并更新 primary；primary 为空仅复用。
func (m *Manager) serveAggregateLLMLocked(primary string) (uint32, error) {
	if m.agentLLMServiceID != 0 {
		if primary != "" {
			m.agentLLMName = primary
		}
		return m.agentLLMServiceID, nil
	}
	m.agentLLMName = primary
	serviceID, err := m.serveAggregateLLMOnBroker(m.broker, primary)
	if err != nil {
		return 0, err
	}
	m.agentLLMServiceID = serviceID
	return serviceID, nil
}

// serveAggregateLLMOnBroker 在指定 broker 上挂载聚合 LLM 服务（provider 请求时
// 动态路由）；返回 serviceID。互通机制 1 中，该服务须挂在本插件 client 的
// broker 上（插件进程经自身 broker.Dial 访问），而不仅是 agent broker。
func (m *Manager) serveAggregateLLMOnBroker(broker *plugin.GRPCBroker, primary string) (uint32, error) {
	if broker == nil {
		return 0, fmt.Errorf("broker not available, cannot serve aggregate LLM service")
	}
	serviceID := broker.NextId()
	go broker.AcceptAndServe(serviceID, func(opts []grpc.ServerOption) *grpc.Server {
		s := grpc.NewServer(opts...)
		proto.RegisterLLMServiceServer(s, &llmAggregateServer{m: m})
		return s
	})
	return serviceID, nil
}

// chatWithProvider 以 provider 执行一次非流式调用并转 proto 响应。
func chatWithProvider(provider LLMProvider, ctx context.Context, req *proto.ChatRequest) (*proto.ChatResponse, error) {
	messages, tools := protoMessagesToPlugin(req)
	resp, err := provider.Chat(ctx, messages, tools, int(req.MaxTokens))
	if err != nil {
		return nil, err
	}
	toolCalls := make([]*proto.ToolCall, len(resp.ToolCalls))
	for i, tc := range resp.ToolCalls {
		toolCalls[i] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: tc.ArgumentsJSON()}
	}
	return &proto.ChatResponse{
		Content:      resp.Content,
		FinishReason: resp.FinishReason,
		ToolCalls:    toolCalls,
	}, nil
}

// chatStreamWithProvider 以 provider 执行流式调用并逐帧转发；
// 真发出过内容帧才标记 call.StreamStarted（此后失败不再切换/重试，避免重复输出），
// 并随帧累计 finish_reason/工具调用数/用量供结算日志。
//
// 为什么按「帧里有内容」而不是「收到过帧」标记：provider 把 HTTP 400 之类**请求阶段**
// 的失败也做成一个带 Error 的帧（`LLM stream error: … prompt is too long: 1268641
// tokens > 1048576 maximum`）。按「收到过帧」标记的话，一个还没吐字的 400 会被判成
// 「已经流式输出过」，于是上层既不重试、也不记超窗收紧——整条自愈路径（4xx → 收紧预算
// → 滑窗 → 重试）被跳过，用户直接看到原始 400。实测就是这么发生的。
func chatStreamWithProvider(provider LLMProvider, req *proto.ChatRequest, stream proto.LLMService_ChatStreamServer, call *LLMCall) error {
	messages, tools := protoMessagesToPlugin(req)
	ch, err := provider.ChatStream(stream.Context(), messages, tools, int(req.MaxTokens))
	if err != nil {
		call.Err = err
		return err
	}
	for item := range ch {
		if item.Error != "" {
			call.Err = fmt.Errorf("LLM stream error: %s", item.Error)
			return call.Err
		}
		if item.FinishReason != "" {
			call.FinishReason = item.FinishReason
		}
		if len(item.ToolCalls) > 0 {
			call.ToolCalls = len(item.ToolCalls)
		}
		if item.Usage != nil {
			call.Usage = item.Usage
		}
		// 只有「真的对外发出过内容」才算流已开始：纯用量帧、空帧都不算。
		if item.Content != "" || item.Reasoning != "" || len(item.ToolCalls) > 0 {
			call.StreamStarted = true
		}
		toolCalls := make([]*proto.ToolCall, len(item.ToolCalls))
		for i, tc := range item.ToolCalls {
			toolCalls[i] = &proto.ToolCall{Id: tc.ID, Name: tc.Name, ArgumentsJson: tc.ArgumentsJSON()}
		}
		if err := stream.Send(&proto.ChatStreamResponse{
			Content:      item.Content,
			FinishReason: item.FinishReason,
			ToolCalls:    toolCalls,
			Usage:        UsageToProto(item.Usage),
			Reasoning:    item.Reasoning,
		}); err != nil {
			call.Err = err
			return err
		}
	}
	return nil
}
