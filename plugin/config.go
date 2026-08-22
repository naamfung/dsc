package plugin

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ConfigPath 默認配置文件路徑，基於程序可執行文件所在目錄
var ConfigPath string

func init() {
	exePath, err := os.Executable()
	if err != nil {
		// 若無法獲取可執行文件路徑，則回退到相對路徑
		ConfigPath = "./config/config.yaml"
	} else {
		execDir := filepath.Dir(exePath)
		ConfigPath = filepath.Join(execDir, "config", "config.yaml")
	}
}

type Config struct {
	WorkspaceRoot string `json:"workspace_root" yaml:"workspace_root"`
	// WorkspaceProtectionEnabled 工作空間保護開關，三值：0=缺省（默認啟用）、-1=顯式關閉、+1=顯式啟用。
	// 缺省值置 0，因此無需指針即可區分「未聲明」與「顯式聲明」。
	WorkspaceProtectionEnabled int           `json:"workspace_protection_enabled" yaml:"workspace_protection_enabled"`
	Mode                       string        `json:"mode" yaml:"mode"` // 默認模式：minimal 或 standard
	Plugins                    []PluginEntry `json:"plugins" yaml:"plugins"`
	// 可保留舊的 LLM 字段便於過渡，但推薦統一使用 Plugins
	DefaultLLM string `json:"default_llm" yaml:"default_llm"`
	// ContextWindow 上下文窗口大小（token 数）；0 表示未配置，
	// 由宿主探测 LLAMACPP 的 /v1/models 獲取 n_ctx，仍失敗則用默認 128K×1024
	ContextWindow int `json:"context_window" yaml:"context_window"`
	// Persona "你是一個…助手" 身份句（預設可配，同 DSH 的 deployment persona）；空則用 DeepSeek 官方默認
	Persona string `json:"persona" yaml:"persona"`
	// PlanSection plan 模式激活时注入 system prompt 的部署方引导文案（同 DSH plan-mode 的 section）；
	// 空则用 react-loop 内置默认（DSH 示例文案）
	PlanSection string `json:"plan_section" yaml:"plan_section"`
}

type PluginEntry struct {
	Name       string `json:"name" yaml:"name"`
	Type       string `json:"type" yaml:"type"` // "llm", "agent", "tool", "dsc"
	BinaryPath string `json:"binary_path" yaml:"binary_path"`
	// 可選：是否啟用、參數等
	Enabled   bool           `json:"enabled" yaml:"enabled" default:"true"`
	DependsOn *PluginDepends `json:"depends_on" yaml:"depends_on"`
	// 可選：傳遞給插件子進程的額外環境變量（合併宿主環境，插件值優先）
	Env map[string]string `json:"env" yaml:"env"`
}

type PluginDepends struct {
	LLM   string   `json:"llm" yaml:"llm"`
	Tools []string `json:"tools" yaml:"tools"`
}

// LoadConfig 從指定路徑加載配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveConfig 將配置保存到指定路徑
func SaveConfig(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	// 確保目錄存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// UpdateWorkspaceProtectionEnabled 更新工作空間保護狀態並保存到配置文件
func UpdateWorkspaceProtectionEnabled(enabled bool, configPath string) error {
	v := -1
	if enabled {
		v = 1
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		// 如果配置文件不存在或加載失敗，創建一個新的配置
		cfg = &Config{
			WorkspaceRoot:              "./workspace",
			WorkspaceProtectionEnabled: v,
			Mode:                       "standard",
			Plugins:                    nil,
			DefaultLLM:                 "",
			ContextWindow:              0,
			Persona:                    "",
		}
	} else {
		cfg.WorkspaceProtectionEnabled = v
	}
	return SaveConfig(configPath, cfg)
}

// UpdateMode 更新模式狀態並保存到配置文件
func UpdateMode(mode string, configPath string) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		// 如果配置文件不存在或加載失敗，創建一個新的配置
		cfg = &Config{
			WorkspaceRoot:              "./workspace",
			WorkspaceProtectionEnabled: 0, // 缺省：默認啟用
			Mode:                       mode,
			Plugins:                    nil,
			DefaultLLM:                 "",
			ContextWindow:              0,
			Persona:                    "",
		}
	} else {
		cfg.Mode = mode
	}
	return SaveConfig(configPath, cfg)
}
