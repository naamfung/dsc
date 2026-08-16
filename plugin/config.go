package plugin

type Config struct {
	LLM struct {
		Provider string `json:"provider" yaml:"provider"`
		Model    string `json:"model" yaml:"model"`
		APIKey   string `json:"api_key" yaml:"api_key"`
		BaseURL  string `json:"base_url" yaml:"base_url"`
	} `json:"llm" yaml:"llm"`
	Agent struct {
		BinaryPath    string `json:"binary_path" yaml:"binary_path"`
		MaxIterations int    `json:"max_iterations" yaml:"max_iterations"`
		MaxMessages   int    `json:"max_messages" yaml:"max_messages"`
	} `json:"agent" yaml:"agent"`
	WorkspaceRoot string `json:"workspace_root" yaml:"workspace_root"`
}
