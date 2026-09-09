package core

import (
        "fmt"
        "strconv"
        "sync"
        "unicode/utf8"
)

// Token 计量服务（对齐 DSH/Cordis 的 ctx.tokenMeter 服务）。
//
// DSH 的 TokenMeter 是一个 Service（TokenMeter extends Service），提供：
//   - measure(session, requestHeader) → TokenMeasurement（压力感知 + 表面 token 计量）
//   - 经 sessionProjections 注册 tokenUsage / contextPressure / contextBreakdown 投影
//   - 重放感知：基于最新成功调用的 provider usage 作为锚点，增量计算
//
// DSC 的适配：DSC 的 agent 是独立 gRPC 插件进程，session 是宿主侧的 event-sourced 日志。
// 宿主侧的 TokenMeter 提供同步的 token 估算与用量追踪，agent 经 admin API 或
// 直接经宿主调用获取计量结果。计量策略对齐 DSH：
//   - 优先使用 provider 上报的 Usage（精确）
//   - provider 不可用或低估（提示缓存命中时 input_tokens 仅含增量）时回退字节级启发式
//   - 提示缓存感知：input_tokens + cache_read_input_tokens 才是完整请求 token
//   - 上下文压力 = 已用 token / 上下文窗口

// TokenMeasurement 一次 token 计量的结果（对齐 DSH TokenMeasurement）。
type TokenMeasurement struct {
        // TotalTokens 当前上下文总 token 数（含 system prompt、历史、工具定义）。
        TotalTokens int `json:"total_tokens"`
        // PromptTokens 最近一次 LLM 请求的 prompt token 数（provider 上报值）。
        PromptTokens int `json:"prompt_tokens"`
        // CompletionTokens 最近一次 LLM 响应的 completion token 数。
        CompletionTokens int `json:"completion_tokens"`
        // CacheReadTokens 提示缓存命中的读取 token 数。
        CacheReadTokens int `json:"cache_read_tokens"`
        // CacheCreationTokens 提示缓存写入的 token 数。
        CacheCreationTokens int `json:"cache_creation_tokens"`
        // ContextWindow 上下文窗口大小（token 数）。
        ContextWindow int `json:"context_window"`
        // Pressure 上下文压力比（0.0 ~ 1.0+），= TotalTokens / ContextWindow。
        Pressure float64 `json:"pressure"`
        // Source 计量来源："usage"（provider 上报）或 "estimate"（启发式估算）。
        Source string `json:"source"`
}

// TokenMeter token 计量服务（对齐 DSH TokenMeter Service）。
// 线程安全；agent 的 ReAct 循环每轮调用 UpdateUsage 更新锚点，
// 调用 Measure 获取当前压力感知。
type TokenMeter struct {
        mu            sync.RWMutex
        lastUsage     *Usage // 最近一次 provider 上报的用量（精确锚点）
        contextWindow int    // 上下文窗口大小
        totalTokens   int    // 当前估算的总 token（用于无 usage 时回退）
}

// NewTokenMeter 创建 token 计量服务。contextWindow 为上下文窗口大小（token 数）。
func NewTokenMeter(contextWindow int) *TokenMeter {
        return &TokenMeter{
                contextWindow: contextWindow,
        }
}

// UpdateUsage 更新最近一次 provider 上报的用量（线程安全）。
// agent 的 ReAct 循环在每次 LLM 响应后调用此方法。
// 提示缓存感知：若 cache_read > 0，实际请求 token = prompt_tokens + cache_read_tokens
// （provider 在缓存命中时只上报增量部分，需加回 cache_read 才是完整请求大小）。
func (tm *TokenMeter) UpdateUsage(usage *Usage) {
        tm.mu.Lock()
        defer tm.mu.Unlock()
        if usage == nil {
                return
        }
        tm.lastUsage = usage
        // 提示缓存感知：prompt_tokens + cache_read_input_tokens = 完整请求 token
        // （对齐 DSC 既有的 llama.cpp 提示缓存感知逻辑）
        fullPrompt := int(usage.PromptTokens)
        if usage.CacheReadInputTokens > 0 {
                fullPrompt += int(usage.CacheReadInputTokens)
        }
        tm.totalTokens = fullPrompt + int(usage.CompletionTokens)
}

// UpdateContextWindow 更新上下文窗口大小（模型探测后可能变化）。
func (tm *TokenMeter) UpdateContextWindow(window int) {
        tm.mu.Lock()
        defer tm.mu.Unlock()
        tm.contextWindow = window
}

// Measure 返回当前 token 计量结果（线程安全）。
// messages 为当前请求的消息列表（供无 usage 时字节级启发式估算回退）。
func (tm *TokenMeter) Measure(messages []*Message) TokenMeasurement {
        tm.mu.RLock()
        defer tm.mu.RUnlock()

        m := TokenMeasurement{
                ContextWindow: tm.contextWindow,
        }

        if tm.lastUsage != nil {
                // 精确路径：provider 上报了 usage
                m.PromptTokens = int(tm.lastUsage.PromptTokens)
                m.CompletionTokens = int(tm.lastUsage.CompletionTokens)
                m.CacheReadTokens = int(tm.lastUsage.CacheReadInputTokens)
                m.CacheCreationTokens = int(tm.lastUsage.CacheCreationInputTokens)
                // 提示缓存感知：完整请求 token = prompt + cache_read
                fullPrompt := m.PromptTokens + m.CacheReadTokens
                m.TotalTokens = fullPrompt + m.CompletionTokens
                m.Source = "usage"
        } else {
                // 回退路径：字节级启发式估算（对齐 DSH estimateMessage）
                m.TotalTokens = EstimateMessagesTokens(messages)
                m.Source = "estimate"
        }

        if tm.contextWindow > 0 {
                m.Pressure = float64(m.TotalTokens) / float64(tm.contextWindow)
        }

        return m
}

// ShouldCompact 判断当前压力是否超过阈值，需要触发压缩。
// thresholdRatio 为触发比例（如 0.80）。
func (tm *TokenMeter) ShouldCompact(thresholdRatio float64) bool {
        tm.mu.RLock()
        defer tm.mu.RUnlock()
        if tm.contextWindow <= 0 || tm.totalTokens <= 0 {
                return false
        }
        return float64(tm.totalTokens)/float64(tm.contextWindow) >= thresholdRatio
}

// EstimateMessagesTokens 估算消息列表的总 token 数（对齐 DSH estimateMessage）。
// 字节级启发式：取「字节数/4」与「rune 数」的较大者——英文按 /4（约 4 字符 1 token），
// CJK（UTF-8 每字 3 字节）字节/4 会低估，故回调为 rune 数（每字按 1 token）。
func EstimateMessagesTokens(messages []*Message) int {
        total := 0
        for _, msg := range messages {
                total += EstimateMessageTokens(msg)
        }
        return total
}

// EstimateMessageTokens 估算单条消息的 token 数（对齐 DSH estimateMessage）。
// 文本估算 + 每条消息的固定结构开销（角色、tool_call_id 等）；
// 工具调用额外计入名称与参数；图像按固定上限估算。
func EstimateMessageTokens(msg *Message) int {
        if msg == nil {
                return 0
        }
        toks := EstimateTextTokens(msg.Content)
        if msg.Role == "tool" {
                toks += 6 // tool 角色 + tool_call_id 开销
        } else {
                toks += 4 // role + delimiter 开销
        }
        for _, img := range msg.Images {
                if img == "" {
                        continue
                }
                toks += 384 // 单图 token 上限（视觉模型按尺寸换算）
        }
        for _, tc := range msg.ToolCalls {
                // 工具调用开销：name + arguments（合并估算避免 name 重复计入）
                argsJSON := fmt.Sprintf("%v", tc.Arguments)
                toks += EstimateTextTokens(tc.Name+argsJSON) + 8
        }
        return toks
}

// EstimateTextTokens 估算一段文本的 token 数。
// 取「字节数/4」与「rune 数」的较大者——英文按 /4（约 4 字符 1 token）；
// CJK（UTF-8 每字 3 字节）字节/4 会低估，故回调为 rune 数（每字按 1 token）。
func EstimateTextTokens(s string) int {
        bytes := len(s)
        runes := utf8.RuneCountInString(s)
        if byBytes := (bytes + 3) / 4; byBytes > runes {
                return byBytes
        }
        return runes
}

// EstimateToolTokens 估算工具定义的 token 数（system prompt 中的工具目录）。
// 每个工具按 name + description + parameters JSON 的字符估算 + 固定结构开销。
func EstimateToolTokens(tools []Tool) int {
        total := 0
        for _, t := range tools {
                toks := EstimateTextTokens(t.Name) + EstimateTextTokens(t.Description) + EstimateTextTokens(t.ParametersJSON)
                toks += 16 // JSON schema 结构开销
                total += toks
        }
        return total
}

// FormatPressure 格式化压力比为人读字符串（如 "78%"）。
func FormatPressure(pressure float64) string {
        pct := int(pressure * 100)
        return strconv.Itoa(pct) + "%"
}
