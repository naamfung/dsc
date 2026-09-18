package dsc

import (
	"testing"
)

func TestEnvStr(t *testing.T) {
	t.Setenv("DSC_TEST_STR", "  hello  ")
	if got := EnvStr("DSC_TEST_STR", "def"); got != "hello" {
		t.Errorf("EnvStr trimmed = %q, want %q", got, "hello")
	}
	t.Setenv("DSC_TEST_STR", "")
	if got := EnvStr("DSC_TEST_STR", "def"); got != "def" {
		t.Errorf("EnvStr absent = %q, want %q", got, "def")
	}
}

func TestEnvInt(t *testing.T) {
	cases := []struct {
		env  string
		want int
	}{
		{"42", 42},
		{"  -7  ", -7},
		{"not-a-number", 99},
		{"", 99},
	}
	for _, c := range cases {
		t.Setenv("DSC_TEST_INT", c.env)
		if got := EnvInt("DSC_TEST_INT", 99); got != c.want {
			t.Errorf("EnvInt(%q) = %d, want %d", c.env, got, c.want)
		}
	}
}

func TestEnvInt64(t *testing.T) {
	t.Setenv("DSC_TEST_I64", "12345678901")
	if got := EnvInt64("DSC_TEST_I64", 0); got != 12345678901 {
		t.Errorf("EnvInt64 = %d, want %d", got, 12345678901)
	}
	t.Setenv("DSC_TEST_I64", "bad")
	if got := EnvInt64("DSC_TEST_I64", 7); got != 7 {
		t.Errorf("EnvInt64 bad = %d, want %d", got, 7)
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		env  string
		want bool
	}{
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"no", false},
		{"off", false},
		{"", true}, // 缺席返回 def
	}
	for _, c := range cases {
		t.Setenv("DSC_TEST_BOOL", c.env)
		if got := EnvBool("DSC_TEST_BOOL", true); got != c.want {
			t.Errorf("EnvBool(%q) = %v, want %v", c.env, got, c.want)
		}
	}
}

func TestEnvPositiveInt(t *testing.T) {
	cases := []struct {
		env  string
		want int
	}{
		{"42", 42},
		{"  100  ", 100},
		{"0", 0},
		{"-5", 0},
		{"bad", 0},
		{"", 0},
	}
	for _, c := range cases {
		t.Setenv("DSC_TEST_POS", c.env)
		if got := EnvPositiveInt("DSC_TEST_POS"); got != c.want {
			t.Errorf("EnvPositiveInt(%q) = %d, want %d", c.env, got, c.want)
		}
	}
}

func TestEnvPositiveInt64(t *testing.T) {
	cases := []struct {
		env  string
		want int64
	}{
		{"9999999999", 9999999999},
		{"0", 0},
		{"-1", 0},
		{"bad", 0},
		{"", 0},
	}
	for _, c := range cases {
		t.Setenv("DSC_TEST_POS64", c.env)
		if got := EnvPositiveInt64("DSC_TEST_POS64"); got != c.want {
			t.Errorf("EnvPositiveInt64(%q) = %d, want %d", c.env, got, c.want)
		}
	}
}

func TestLoadLLMConfig(t *testing.T) {
	// 显式 provider env
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.example.com")
	t.Setenv("ANTHROPIC_MODEL", "claude-test")
	t.Setenv("ANTHROPIC_MAX_OUTPUT_TOKENS", "8192")
	t.Setenv("DSC_NO_VISION", "")
	cfg := LoadLLMConfig("anthropic")
	if cfg.APIKey != "sk-test" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "sk-test")
	}
	if cfg.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://api.example.com")
	}
	if cfg.Model != "claude-test" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-test")
	}
	if cfg.MaxOutputTokens != 8192 {
		t.Errorf("MaxOutputTokens = %d, want %d", cfg.MaxOutputTokens, 8192)
	}
	if !cfg.VisionEnabled {
		t.Error("VisionEnabled = false, want true")
	}

	// DSC_MAX_OUTPUT_TOKENS 兜底
	t.Setenv("ANTHROPIC_MAX_OUTPUT_TOKENS", "")
	t.Setenv("DSC_MAX_OUTPUT_TOKENS", "4096")
	cfg = LoadLLMConfig("anthropic")
	if cfg.MaxOutputTokens != 4096 {
		t.Errorf("MaxOutputTokens fallback = %d, want %d", cfg.MaxOutputTokens, 4096)
	}

	// DSC_NO_VISION=1 关闭视觉
	t.Setenv("DSC_NO_VISION", "1")
	cfg = LoadLLMConfig("anthropic")
	if cfg.VisionEnabled {
		t.Error("VisionEnabled = true, want false (DSC_NO_VISION=1)")
	}

	// OpenAI provider
	t.Setenv("OPENAI_API_KEY", "sk-openai")
	t.Setenv("OPENAI_BASE_URL", "https://api.openai.com/v1")
	cfg = LoadLLMConfig("openai")
	if cfg.APIKey != "sk-openai" {
		t.Errorf("OpenAI APIKey = %q, want %q", cfg.APIKey, "sk-openai")
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("OpenAI BaseURL = %q, want %q", cfg.BaseURL, "https://api.openai.com/v1")
	}
}
