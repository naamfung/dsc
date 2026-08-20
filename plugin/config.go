package plugin

type Config struct {
	WorkspaceRoot string        `json:"workspace_root" yaml:"workspace_root"`
	Plugins       []PluginEntry `json:"plugins" yaml:"plugins"`
	// 可保留舊的 LLM 字段便於過渡，但推薦統一使用 Plugins
	DefaultLLM    string        `json:"default_llm" yaml:"default_llm"`
	// ContextWindow 上下文窗口大小（token 数）；0 表示未配置，
	// 由宿主探测 LLAMACPP 的 /v1/models 獲取 n_ctx，仍失敗則用默認 128K×1024
	ContextWindow int `json:"context_window" yaml:"context_window"`
	// Persona "你是一個…助手" 身份句（預設可配，同 DSH 的 deployment persona）；空則用 DeepSeek 官方默認
	Persona string `json:"persona" yaml:"persona"`
}

type PluginEntry struct {
	Name       string         `json:"name" yaml:"name"`
	Type       string         `json:"type" yaml:"type"` // "llm", "agent", "tool", "dsc"
	BinaryPath string         `json:"binary_path" yaml:"binary_path"`
	// 可選：是否啟用、參數等
	Enabled   bool            `json:"enabled" yaml:"enabled" default:"true"`
	DependsOn *PluginDepends  `json:"depends_on" yaml:"depends_on"`
	// 可選：傳遞給插件子進程的額外環境變量（合併宿主環境，插件值優先）
	Env map[string]string `json:"env" yaml:"env"`
}

type PluginDepends struct {
	LLM   string   `json:"llm" yaml:"llm"`
	Tools []string `json:"tools" yaml:"tools"`
}
