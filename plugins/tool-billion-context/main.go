// Package main 实现 tool-billion-context 插件：以 acp-kernel 算法接管 DSC 的上下文压缩。
//
// 接管方式（零 agent 改动）：
//  1. 注册 Hook.OnEvent 订阅 agent/pre-step 事件
//  2. 每次进入 LLM 请求前，插件收到当前消息列表 JSON
//  3. 插件经 bcacp.ProcessTurn 跑 pipeline：分配 ref、注入 <acp> 标签、应用 prune、决策 nudge
//  4. 返回改写后的消息列表（{"messages": [...]}）给宿主，宿主透传给 LLM
//  5. 模型调用 compress 工具时触发压缩：插件调 bcacp.ApplyCompression 持久化块、遮蔽范围
//
// 4 个模型可见工具：
//   - compress: 把消息范围替换为摘要（模型生成 summary）
//   - decompress: 恢复压缩块内容
//   - search_context: 在压缩块摘要中搜索关键词
//   - acp_status: 查看上下文使用率与可压缩范围
//
// state 持久化：每个 session 一个 JSON 文件（ExecDir/billion-context/<session>.json）。
// fork/恢复时经 session 事件日志重放重建（后续实现）。
package main

import (
        "context"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"

        "dsc/core"
        dsc "dsc-sdk"

        bcacp "dsc-plugin-tool-billion-context/acp"
        bcadapter "dsc-plugin-tool-billion-context/adapter"
        "dsc/proto"
)

// BillionContext 插件主结构。
type BillionContext struct {
        mu        sync.Mutex
        store     *bcadapter.StateStore
        config    bcacp.Config
        sessionID string // 当前会话 ID（经环境变量 DSC_SESSION_ID 注入）

        // lastMessages 缓存最近一次 pre-step 收到的 CoreMessage 列表，供 handleCompress
        // 解析 mNNNNN 边界引用（解决"compress 工具调用时无消息列表"的致命 bug）。
        // 每次 handlePreStep 更新此缓存；handleCompress 从中读取。
        lastMessages []bcacp.CoreMessage
}

func main() {
        // 从环境变量读取配置
        execDir := os.Getenv("DSC_EXEC_DIR")
        if execDir == "" {
                execDir, _ = os.Getwd()
        }
        stateDir := filepath.Join(execDir, "billion-context")
        store := bcadapter.NewStateStore(stateDir)

        // 上下文窗口（默认 128K，可经 DSC_CONTEXT_WINDOW 覆盖）
        contextWindow := 131072
        if w := os.Getenv("DSC_CONTEXT_WINDOW"); w != "" {
                if n, err := parseInt(w); err == nil && n > 0 {
                        contextWindow = n
                }
        }

        bc := &BillionContext{
                store:  store,
                config: bcacp.DefaultConfig(contextWindow),
        }

        sdkInst := dsc.New(dsc.Config{
                Name:    "tool-billion-context",
                Version: "0.1.0",
                Type:    dsc.TypeDsc,
                // 声明提供 "compaction" 能力：宿主检测到任何插件 Provides compaction 后，
                // 自动设 DSC_ACP_ACTIVE=1 让 agent-react-loop 跳过内联 compactHistory。
                // 对齐 DSH preset 不挂 compaction-basic 改挂 billion-context 的后端替换模式——
                // 不硬编码插件名，任何声明 Provides compaction 的插件都接管。
                Provides: map[string]string{
                        "compaction": "true",
                },
        })

        // 注册 4 个工具
        sdkInst.Tool(dsc.Tool{
                Name:        "compress",
                Description: "Replace a contiguous range of older conversation with a detailed summary you write. Use when content is genuinely consumed (no longer needed for the current task step). Batch form: content=[{startId,endId,summary,topic?}].",
                Handler:     bc.handleCompress,
        })
        sdkInst.Tool(dsc.Tool{
                Name:        "decompress",
                Description: "Restore a previously compressed block's content. Use when you need exact details lost in compression.",
                Handler:     bc.handleDecompress,
        })
        sdkInst.Tool(dsc.Tool{
                Name:        "search_context",
                Description: "Search through compressed block summaries by keyword. Use BEFORE decompressing to find the right block.",
                Handler:     bc.handleSearch,
        })
        sdkInst.Tool(dsc.Tool{
                Name:        "acp_status",
                Description: "Show context usage and compressible ranges. No args = overview. Use to find what to compress next.",
                Handler:     bc.handleStatus,
        })

        // Hook: 拦截 agent/pre-step 改写消息列表
        sdkInst.Hook(dsc.Hook{
                OnEvent: bc.onEvent,
        })

        sdkInst.Serve()
}

// onEvent 处理宿主事件。关心 agent/pre-step（改写消息列表）；
// 其余事件忽略（返回空 result，不影响宿主）。
func (bc *BillionContext) onEvent(ctx context.Context, eventType, dataJSON string) (string, error) {
        switch eventType {
        case string(core.EventAgentPreStep):
                return bc.handlePreStep(ctx, dataJSON)
        default:
                return "", nil
        }
}

// handlePreStep 处理 agent/pre-step 事件：跑 acp pipeline，返回改写后的消息列表。
//
// 协议：dataJSON 为 AgentPreStepEvent 的 JSON（含 MessagesJSON 字段）
// 返回：resultJSON 为 {"messages": [...]} 形式（改写后的 proto.Message 数组 JSON）
func (bc *BillionContext) handlePreStep(ctx context.Context, dataJSON string) (string, error) {
        var ev core.AgentPreStepEvent
        if err := json.Unmarshal([]byte(dataJSON), &ev); err != nil {
                return "", fmt.Errorf("billion-context: parse pre-step event: %w", err)
        }

        // 解析消息列表
        var protoMsgs []*proto.Message
        if ev.MessagesJSON != "" {
                if err := json.Unmarshal([]byte(ev.MessagesJSON), &protoMsgs); err != nil {
                        return "", fmt.Errorf("billion-context: parse messages: %w", err)
                }
        }

        // 加载（或创建）session state
        sessionID := bc.getSessionID()
        state, err := bc.store.Load(sessionID)
        if err != nil {
                return "", fmt.Errorf("billion-context: load state: %w", err)
        }

        // 转换为 acp 核心消息格式
        coreMsgs := bcadapter.ToCoreMessages(protoMsgs, bc.msgIDProvider())

        // 缓存消息列表供 handleCompress 使用（解决"compress 工具调用时无消息列表"的致命 bug）
        bc.mu.Lock()
        bc.lastMessages = coreMsgs
        bc.mu.Unlock()

        // token 计数：优先用事件携带的宿主估算值（含工具结构开销），回退本地估算
        tokenCount := ev.TokenCount
        if tokenCount <= 0 {
                tokenCount = estimateTokens(protoMsgs)
        }

        // 跑 pipeline（renderTags=false：不注入 <acp> 标签到消息文本——标签的 tokens
        // 属性每轮估算会变化，破坏前缀缓存。改为仅用 mNNNNN ref 映射，标签信息经
        // acp_status 工具按需查询）
        result := bcacp.ProcessTurn(coreMsgs, state, bc.config, tokenCount)

        // 持久化更新后的 state
        if err := bc.store.Save(sessionID, result.State); err != nil {
                // 持久化失败不阻塞请求（已记内存）
                fmt.Fprintf(os.Stderr, "[billion-context] save state failed: %v\n", err)
        }

        // 注入 ACP system prompt（对齐 DSH systemPrompt.section 机制）
        //
        // 前缀缓存稳定性关键设计：
        //  - 不插入新的 system 消息（会改变消息数量与位置，破坏前缀缓存）
        //  - 而是把 ACP 指导文本追加到第一条 system 消息的末尾（合并为一条消息）
        //  - ACP 指导文本每轮完全一样 → 合并后的 system 消息内容稳定 → 前缀缓存命中
        //  - 仅在首次注入时改变 system 消息内容（增加 ACP 段落），后续轮次文本不变
        //
        // nudge 处理（前缀缓存友好的方式）：
        //  - nudge 文本每次不同（含 usage%、ranges），不能放在前缀位置
        //  - 放在消息列表尾部（最后一条消息之后）作为临时 user 消息
        //  - 尾部追加不影响前缀，且 nudge 仅在需要压缩时出现
        renderedMsgs := result.Messages
        if len(renderedMsgs) > 0 && renderedMsgs[0].Role == bcacp.RoleSystem {
                // 合并到已有 system 消息：追加 ACP 段落
                // 用 marker 检测是否已注入过（避免重复追加）
                if !containsStr(renderedMsgs[0].Text, "[ACP System Prompt]") {
                        renderedMsgs[0].Text = renderedMsgs[0].Text + "\n\n[ACP System Prompt]\n" + bcacp.ACPSystemPrompt
                }
        } else {
                // 无 system 消息：在头部插入（首次创建，后续轮次稳定）
                renderedMsgs = append([]bcacp.CoreMessage{{
                        ID:          "acp_system_prompt",
                        Role:        bcacp.RoleSystem,
                        ContentType: bcacp.ContentTypeText,
                        Text:        bcacp.ACPSystemPrompt,
                }}, renderedMsgs...)
        }

        // nudge 作为尾部追加的 user 消息（不影响前缀缓存）
        if result.Nudge != nil && result.Nudge.ShouldInject {
                nudgeText := bcacp.FormatNudgeText(*result.Nudge)
                if nudgeText != "" {
                        renderedMsgs = append(renderedMsgs, bcacp.CoreMessage{
                                ID:          "acp_nudge",
                                Role:        bcacp.RoleUser,
                                ContentType: bcacp.ContentTypeText,
                                Text:        nudgeText,
                        })
                }
        }

        // 转回 proto.Message
        rewrittenProto := bcadapter.FromCoreMessages(renderedMsgs)
        rewrittenJSON, _ := json.Marshal(rewrittenProto)

        // 返回改写后的消息列表（宿主会用此替换原消息列表发给 LLM）
        return fmt.Sprintf(`{"messages": %s}`, string(rewrittenJSON)), nil
}

// handleCompress 处理 compress 工具调用。
func (bc *BillionContext) handleCompress(ctx context.Context, args json.RawMessage) (string, error) {
        var req struct {
                Content []struct {
                        StartRef string `json:"startId"`
                        EndRef   string `json:"endId"`
                        Summary  string `json:"summary"`
                        Topic    string `json:"topic,omitempty"`
                } `json:"content"`
        }
        if err := json.Unmarshal(args, &req); err != nil {
                return "", fmt.Errorf("invalid args: %w", err)
        }
        if len(req.Content) == 0 {
                return "", fmt.Errorf("content is required (at least one range)")
        }

        sessionID := bc.getSessionID()
        state, err := bc.store.Load(sessionID)
        if err != nil {
                return "", fmt.Errorf("load state: %w", err)
        }

        // 把请求转为 acp PruneRange 列表
        ranges := make([]bcacp.PruneRange, 0, len(req.Content))
        for _, r := range req.Content {
                if r.StartRef == "" || r.EndRef == "" || r.Summary == "" {
                        continue
                }
                ranges = append(ranges, bcacp.PruneRange{
                        StartRef: r.StartRef,
                        EndRef:   r.EndRef,
                        Summary:  r.Summary,
                        Topic:    r.Topic,
                })
        }
        if len(ranges) == 0 {
                return "", fmt.Errorf("no valid ranges (each requires startId, endId, summary)")
        }

        // 从缓存获取最近一次 pre-step 的消息列表（解决"compress 工具调用时无消息列表"的致命 bug）
        bc.mu.Lock()
        cachedMsgs := bc.lastMessages
        bc.mu.Unlock()
        if len(cachedMsgs) == 0 {
                return "", fmt.Errorf("no cached messages: compress must be called after at least one LLM request (pre-step event)")
        }

        _, result := bcacp.ApplyCompression(ranges, cachedMsgs, state, bc.config)

        // 持久化
        if err := bc.store.Save(sessionID, state); err != nil {
                return "", fmt.Errorf("save state: %w", err)
        }

        out, _ := json.Marshal(map[string]any{
                "ok":             true,
                "blocksCreated":  result.BlocksCreated,
                "tokensCompressed": result.TokensCompressed,
                "errors":          result.Errors,
        })
        return string(out), nil
}

// handleDecompress 处理 decompress 工具调用。
func (bc *BillionContext) handleDecompress(ctx context.Context, args json.RawMessage) (string, error) {
        var req struct {
                BlockID string `json:"blockId"`
                Full    bool   `json:"full,omitempty"`
        }
        if err := json.Unmarshal(args, &req); err != nil {
                return "", fmt.Errorf("invalid args: %w", err)
        }
        if req.BlockID == "" {
                return "", fmt.Errorf("blockId is required")
        }

        sessionID := bc.getSessionID()
        state, err := bc.store.Load(sessionID)
        if err != nil {
                return "", fmt.Errorf("load state: %w", err)
        }

        summary, err := bcacp.Decompress(req.BlockID, state, req.Full)
        if err != nil {
                return "", fmt.Errorf("decompress: %w", err)
        }

        out, _ := json.Marshal(map[string]any{
                "ok":      true,
                "blockId": req.BlockID,
                "summary":  summary,
        })
        return string(out), nil
}

// handleSearch 处理 search_context 工具调用。
func (bc *BillionContext) handleSearch(ctx context.Context, args json.RawMessage) (string, error) {
        var req struct {
                Query string `json:"query"`
                Limit int    `json:"limit,omitempty"`
        }
        if err := json.Unmarshal(args, &req); err != nil {
                return "", fmt.Errorf("invalid args: %w", err)
        }
        if req.Query == "" {
                return "", fmt.Errorf("query is required")
        }
        if req.Limit <= 0 {
                req.Limit = 5
        }

        sessionID := bc.getSessionID()
        state, err := bc.store.Load(sessionID)
        if err != nil {
                return "", fmt.Errorf("load state: %w", err)
        }

        results := bcacp.Search(req.Query, state, req.Limit)
        out, _ := json.Marshal(map[string]any{
                "ok":      true,
                "query":   req.Query,
                "results": results,
                "count":   len(results),
        })
        return string(out), nil
}

// handleStatus 处理 acp_status 工具调用。
func (bc *BillionContext) handleStatus(ctx context.Context, args json.RawMessage) (string, error) {
        sessionID := bc.getSessionID()
        state, err := bc.store.Load(sessionID)
        if err != nil {
                return "", fmt.Errorf("load state: %w", err)
        }

        // 估算当前 token 数（无消息列表时用 state 估算）
        tokenCount := state.Stats.TokensCompressed + 1000 // 粗略估算
        report := bcacp.BuildStatus(nil, state, bc.config, tokenCount)
        text := bcacp.FormatStatus(report)

        out, _ := json.Marshal(map[string]any{
                "ok":     true,
                "report": text,
                "usage":  report.ContextUsage,
        })
        return string(out), nil
}

// getSessionID 获取当前会话 ID。
// 优先级：环境变量 DSC_SESSION_ID > "default"
func (bc *BillionContext) getSessionID() string {
        if id := os.Getenv("DSC_SESSION_ID"); id != "" {
                return id
        }
        return "default"
}

// msgIDProvider 提供消息 ID 生成器。
// 用 role + index 生成稳定 ID（同一次请求内消息顺序固定）。
func (bc *BillionContext) msgIDProvider() func(idx int, msg *proto.Message) string {
        return func(idx int, msg *proto.Message) string {
                // 用消息内容 hash 的前 8 位 + index 作为稳定 ID
                // 避免完全相同内容的消息 ID 冲突
                return fmt.Sprintf("msg-%d", idx)
        }
}

// estimateTokens 估算 proto.Message 列表的总 token 数。
func estimateTokens(msgs []*proto.Message) int {
        total := 0
        for _, m := range msgs {
                total += len(m.Content) / 4
                if len(m.ToolCalls) > 0 {
                        total += 8 // tool-call 结构开销
                }
        }
        return total
}

// containsStr 报告 s 是否包含 sub（轻量实现，避免引入 strings 包）。
func containsStr(s, sub string) bool {
        return len(s) >= len(sub) && strings.Contains(s, sub)
}

// parseInt 轻量 string → int。
func parseInt(s string) (int, error) {
        s = strings.TrimSpace(s)
        if s == "" {
                return 0, fmt.Errorf("empty")
        }
        n := 0
        for i := 0; i < len(s); i++ {
                c := s[i]
                if c < '0' || c > '9' {
                        return 0, fmt.Errorf("invalid digit %q", c)
                }
                n = n*10 + int(c-'0')
        }
        return n, nil
}
