// Package adapter 把 DSC 的 proto.Message 与 acp CoreMessage 互转。
// 同时负责 state 持久化（JSON 文件存储）。
package adapter

import (
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "sync"

        bcacp "dsc-plugin-tool-billion-context/acp"
        "dsc/proto"
)

// ToCoreMessages 把 DSC proto.Message 列表转为 acp CoreMessage 列表。
// ID 由调用方提供（通常为消息在 session 中的 seq 或 hash）。
//
// 转换规则：
//   - role 直接映射（user/assistant/system/tool）
//   - contentType 由 role + 是否有 ToolCalls 推断
//   - text 取 Content 字段
//   - toolName / toolCallId 取自 ToolCallId / 第一个 ToolCall.Name
func ToCoreMessages(protoMsgs []*proto.Message, idProvider func(idx int, msg *proto.Message) string) []bcacp.CoreMessage {
        if idProvider == nil {
                idProvider = defaultIDProvider
        }
        out := make([]bcacp.CoreMessage, 0, len(protoMsgs))
        for i, pm := range protoMsgs {
                cm := bcacp.CoreMessage{
                        ID:   idProvider(i, pm),
                        Role: bcacp.MessageRole(pm.Role),
                        Text: pm.Content,
                }
                // 推断 contentType
                switch pm.Role {
                case "tool":
                        cm.ContentType = bcacp.ContentTypeToolResult
                        cm.ToolCallID = pm.ToolCallId
                case "assistant":
                        if len(pm.ToolCalls) > 0 {
                                cm.ContentType = bcacp.ContentTypeToolCall
                                cm.ToolName = pm.ToolCalls[0].Name
                                cm.ToolCallID = pm.ToolCalls[0].Id
                        } else {
                                cm.ContentType = bcacp.ContentTypeText
                        }
                case "system":
                        cm.ContentType = bcacp.ContentTypeText
                default: // user
                        cm.ContentType = bcacp.ContentTypeText
                }
                out = append(out, cm)
        }
        return out
}

// FromCoreMessages 把 acp CoreMessage 列表转回 DSC proto.Message。
// 用于把改写后的消息列表发回给 LLM。
func FromCoreMessages(coreMsgs []bcacp.CoreMessage) []*proto.Message {
        out := make([]*proto.Message, 0, len(coreMsgs))
        for _, cm := range coreMsgs {
                pm := &proto.Message{
                        Role:    string(cm.Role),
                        Content: cm.Text,
                }
                if cm.ToolCallID != "" {
                        pm.ToolCallId = cm.ToolCallID
                }
                out = append(out, pm)
        }
        return out
}

// defaultIDProvider 默认 ID 提供者：用 role + index 生成稳定 ID。
// 调用方可覆盖以使用 session seq 或 content hash。
func defaultIDProvider(idx int, msg *proto.Message) string {
        return fmt.Sprintf("msg-%d", idx)
}

// StateStore 负责 acp state 的持久化（JSON 文件）。
// 每个 session 一个 state 文件，路径为 dir/<sessionID>.json。
type StateStore struct {
        mu   sync.Mutex
        dir  string
        cache map[string]*bcacp.CompressionState // sessionID → state
}

// NewStateStore 创建状态存储。dir 为存储目录（通常 ExecDir/billion-context）。
func NewStateStore(dir string) *StateStore {
        return &StateStore{
                dir:   dir,
                cache: map[string]*bcacp.CompressionState{},
        }
}

// Load 加载（或创建）指定 session 的 state。
func (s *StateStore) Load(sessionID string) (*bcacp.CompressionState, error) {
        s.mu.Lock()
        defer s.mu.Unlock()
        if st, ok := s.cache[sessionID]; ok {
                return st, nil
        }
        path := s.path(sessionID)
        data, err := os.ReadFile(path)
        if err != nil {
                if os.IsNotExist(err) {
                        st := bcacp.CreateInitialState()
                        s.cache[sessionID] = st
                        return st, nil
                }
                return nil, fmt.Errorf("load acp state: %w", err)
        }
        var st bcacp.CompressionState
        if err := json.Unmarshal(data, &st); err != nil {
                // 损坏的 state 文件：重建（避免阻塞会话）
                st = *bcacp.CreateInitialState()
        } else {
                // 确保map初始化（旧版本可能缺少字段）
                if st.MessageRefs.ByRaw == nil {
                        st.MessageRefs.ByRaw = map[string]string{}
                }
                if st.MessageRefs.ByRef == nil {
                        st.MessageRefs.ByRef = map[string]string{}
                }
                if st.Nudge.Anchors == nil {
                        st.Nudge.Anchors = map[string]any{}
                }
        }
        s.cache[sessionID] = &st
        return &st, nil
}

// Save 持久化指定 session 的 state。
func (s *StateStore) Save(sessionID string, state *bcacp.CompressionState) error {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.cache[sessionID] = state
        if err := os.MkdirAll(s.dir, 0700); err != nil {
                return fmt.Errorf("create acp state dir: %w", err)
        }
        data, err := json.MarshalIndent(state, "", "  ")
        if err != nil {
                return fmt.Errorf("marshal acp state: %w", err)
        }
        return os.WriteFile(s.path(sessionID), data, 0600)
}

// path 返回 session state 文件路径。
func (s *StateStore) path(sessionID string) string {
        // 防止路径遍历：sessionID 仅允许字母数字与 -_
        safe := sanitizeSessionID(sessionID)
        return filepath.Join(s.dir, safe+".json")
}

// sanitizeSessionID 把 sessionID 限制为安全字符（防止路径遍历）。
func sanitizeSessionID(id string) string {
        out := make([]byte, 0, len(id))
        for i := 0; i < len(id); i++ {
                c := id[i]
                if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
                        (c >= '0' && c <= '9') || c == '-' || c == '_' {
                        out = append(out, c)
                } else {
                        out = append(out, '_')
                }
        }
        if len(out) == 0 {
                return "default"
        }
        return string(out)
}
