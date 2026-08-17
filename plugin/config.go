package plugin

type Config struct {
	WorkspaceRoot string        `json:"workspace_root" yaml:"workspace_root"`
	Plugins       []PluginEntry `json:"plugins" yaml:"plugins"`
	// 可保留舊的 LLM 字段便於過渡，但推薦統一使用 Plugins
	DefaultLLM    string        `json:"default_llm" yaml:"default_llm"`
}

type PluginEntry struct {
	Name       string         `json:"name" yaml:"name"`
	Type       string         `json:"type" yaml:"type"` // "llm", "agent", "tool", "dsc"
	BinaryPath string         `json:"binary_path" yaml:"binary_path"`
	// 可選：是否啟用、參數等
	Enabled   bool           `json:"enabled" yaml:"enabled" default:"true"`
	DependsOn *PluginDepends `json:"depends_on" yaml:"depends_on"`
}

type PluginDepends struct {
	LLM   string   `json:"llm" yaml:"llm"`
	Tools []string `json:"tools" yaml:"tools"`
}
