package dsc

import (
	"os"
	"strconv"
	"strings"
)

// 通用 env helper：把「读 env + 解析 + 兜底」三步合一，消除各插件散落的
// `if v := os.Getenv(...); v != "" { if n, err := strconv.Atoi(v); err == nil && n > 0 { ... } }`
// 模式。所有 helper 容忍前后空白；解析失败或非法值回落 def，不报错（env 是
// 用户输入，按 fail-soft 处理；需要 fail-loud 的调用方自行校验返回值）。

// EnvStr 读取字符串 env；缺席返回 def。
func EnvStr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// EnvInt 读取 int env；缺席/非法返回 def。
func EnvInt(key string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return def
	}
	return v
}

// EnvInt64 读取 int64 env；缺席/非法返回 def。
func EnvInt64(key string, def int64) int64 {
	v, err := strconv.ParseInt(strings.TrimSpace(os.Getenv(key)), 10, 64)
	if err != nil {
		return def
	}
	return v
}

// EnvBool 读取 bool env；空串/0/false/no/off 返回 false，其余非空返回 true。
// 显式 true/1/yes/on 与隐式非空都视为 true（与既有 envBool 语义一致）。
func EnvBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	}
	return def
}

// EnvPositiveInt 读取正整数 env（>0）；缺席/非法/非正返回 0。
// 对齐既有插件散落的 `parsePositiveInt` 模式。
func EnvPositiveInt(key string) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// EnvPositiveInt64 读取正 int64 env（>0）；缺席/非法/非正返回 0。
// 对齐既有插件散落的 `parsePositiveInt64` 模式。
func EnvPositiveInt64(key string) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
