package dsc

import (
	"os"
	"strings"
)

// LLMConfig 是 LLM 插件的通用配置面，把 apiKey / baseURL / model / maxTokens /
// visionEnabled 五个散落各 LLM 插件 main() 的 env 解析合一。provider 参数取
// "anthropic" / "openai" / "ollama" 等，对应 <PROVIDER>_API_KEY /
// <PROVIDER>_BASE_URL / <PROVIDER>_MODEL / <PROVIDER>_MAX_OUTPUT_TOKENS env；
// DSC_MAX_OUTPUT_TOKENS 与 DSC_NO_VISION 是跨 provider 的宿主通用 env。
//
// 解析顺序（先显式 provider env，后宿主通用 DSC_ env）对齐既有 llm-anthropic
// 与 llm-openai 的实现（maxTokensFromEnv 先读 ANTHROPIC_MAX_OUTPUT_TOKENS，
// 回退 DSC_MAX_OUTPUT_TOKENS），消除两份复制粘贴。
type LLMConfig struct {
	// APIKey 来自 <PROVIDER>_API_KEY；缺席返回空串（由调用方决定是否兜底）。
	APIKey string
	// BaseURL 来自 <PROVIDER>_BASE_URL；缺席返回空串。
	BaseURL string
	// Model 来自 <PROVIDER>_MODEL；缺席返回空串。
	Model string
	// MaxOutputTokens 来自 <PROVIDER>_MAX_OUTPUT_TOKENS，回退 DSC_MAX_OUTPUT_TOKENS；
	// 均缺席或非法返回 0（请求不携带 max_tokens，由模型自然结束）。
	MaxOutputTokens int64
	// VisionEnabled 按 DSC_NO_VISION 强制关闭；否则返回 true（由调用方按模型
	// 能力进一步判定，见 core.ModelSupportsImages）。
	VisionEnabled bool
}

// LoadLLMConfig 按 provider 名读取 LLM 通用配置。provider 不区分大小写。
//
// 用法（LLM 插件 main()）：
//
//	cfg := dsc.LoadLLMConfig("anthropic")
//	// 用 cfg.APIKey / cfg.BaseURL / cfg.Model / cfg.MaxOutputTokens / cfg.VisionEnabled
//
// 对齐 DSH packages/api/llm-* 各 provider 共用配置加载模式。
func LoadLLMConfig(provider string) LLMConfig {
	p := strings.ToUpper(strings.TrimSpace(provider))
	prefix := p + "_"
	cfg := LLMConfig{
		APIKey:          strings.TrimSpace(os.Getenv(prefix + "API_KEY")),
		BaseURL:         strings.TrimSpace(os.Getenv(prefix + "BASE_URL")),
		Model:           strings.TrimSpace(os.Getenv(prefix + "MODEL")),
		MaxOutputTokens: EnvPositiveInt64(prefix + "MAX_OUTPUT_TOKENS"),
		VisionEnabled:   !envNoVision(),
	}
	// DSC_MAX_OUTPUT_TOKENS 兜底（provider 显式优先）
	if cfg.MaxOutputTokens == 0 {
		cfg.MaxOutputTokens = EnvPositiveInt64("DSC_MAX_OUTPUT_TOKENS")
	}
	return cfg
}

// envNoVision 解析 DSC_NO_VISION：1/true/on/yes 返回 true（强制关闭视觉）。
func envNoVision() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DSC_NO_VISION"))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}
