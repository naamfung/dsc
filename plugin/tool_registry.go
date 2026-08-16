package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// ToolRegistry 管理所有可用工具
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolDefinition
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolDefinition),
	}
}

// Register 註冊工具（線程安全）
func (r *ToolRegistry) Register(t ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tool %s already registered", t.Name())
	}
	r.tools[t.Name()] = t
	return nil
}

// Get 獲取工具
func (r *ToolRegistry) Get(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List 返回所有工具名
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// ToOpenAITools 轉換為 OpenAI 工具定義格式
// 用於發送給 LLM Provider
func (r *ToolRegistry) ToOpenAITools() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []map[string]interface{}
	for _, t := range r.tools {
		var params map[string]interface{}
		_ = json.Unmarshal(t.ParametersSchema(), &params)
		result = append(result, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name(),
				"description": t.Description(),
				"parameters":  params,
			},
		})
	}
	return result
}

// Execute 執行工具調用
func (r *ToolRegistry) Execute(ctx context.Context, call PluginToolCall) ToolResult {
	tool, ok := r.Get(call.Name)
	if !ok {
		return ToolResult{
			ToolCallID: call.ID,
			Error:      fmt.Sprintf("unknown tool: %s", call.Name),
		}
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return ToolResult{
			ToolCallID: call.ID,
			Error:      err.Error(),
		}
	}
	return ToolResult{
		ToolCallID: call.ID,
		Content:    result,
	}
}
