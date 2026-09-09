package core

import (
        "context"
        "fmt"
        "os"
        "path/filepath"
        "sync"
)

// 凭据服务（对齐 DSH/Cordis 的 ctx.credentials 服务）。
//
// DSH 的 credentials 是一个 Service，提供：
//   - 按名引用解析：插件经 capability name 声明「我需要 deepseek_api_key」，
//     credentials 服务解析到实际值（env var / 文件 / OAuth token）
//   - 每插件持久化凭据记录：插件的凭据独立存储，不混入 config.yaml 明文
//   - OAuth 授权流程（credentials-authorization 包）
//
// DSC 的适配：DSC 当前用 env whitelist（LLM 插件放行 *_API_KEY 等，其余过滤）。
// 本凭据服务在此基础上增加：
//   - 按名引用解析：插件经 Requires 声明凭据需求，宿主解析到实际值
//   - 持久化凭据存储：落点 ExecDir/credentials/，按插件名隔离
//   - 不存明文到 config.yaml（与 DSH 的「no-secrets-in-config」一致）

// CredentialStore 凭据存储服务（对齐 DSH ctx.credentials）。
// 线程安全；凭据落点 ExecDir/credentials/<plugin_name>.json。
type CredentialStore struct {
        mu    sync.RWMutex
        dir   string
        cache map[string]map[string]string // pluginName -> key -> value
}

// NewCredentialStore 创建凭据存储。dir 为凭据文件目录（通常 ExecDir/credentials）。
func NewCredentialStore(dir string) *CredentialStore {
        return &CredentialStore{
                dir:   dir,
                cache: make(map[string]map[string]string),
        }
}

// Get 按插件名 + 凭据键获取值。
// 查找顺序：1. 内存缓存 2. 凭据文件 3. 环境变量（对齐 DSH 的 fallback 链）。
func (cs *CredentialStore) Get(ctx context.Context, pluginName, key string) (string, error) {
        cs.mu.RLock()
        if pluginCreds, ok := cs.cache[pluginName]; ok {
                if v, ok := pluginCreds[key]; ok {
                        cs.mu.RUnlock()
                        return v, nil
                }
        }
        cs.mu.RUnlock()

        // 尝试从文件加载
        if creds, err := cs.loadFromFile(pluginName); err == nil && creds != nil {
                cs.mu.Lock()
                cs.cache[pluginName] = creds
                cs.mu.Unlock()
                if v, ok := creds[key]; ok {
                        return v, nil
                }
        }

        // 回退到环境变量（对齐 DSC 既有的 env whitelist 机制）
        // 环境变量名约定：PLUGIN_NAME_KEY（全大写、连字符转下划线）
        envKey := credEnvKey(pluginName, key)
        if v := os.Getenv(envKey); v != "" {
                return v, nil
        }

        return "", fmt.Errorf("credential %q for plugin %q not found (checked file + env %s)", key, pluginName, envKey)
}

// Set 设置插件凭据（内存 + 持久化到文件）。
func (cs *CredentialStore) Set(ctx context.Context, pluginName, key, value string) error {
        cs.mu.Lock()
        defer cs.mu.Unlock()

        if cs.cache[pluginName] == nil {
                cs.cache[pluginName] = make(map[string]string)
        }
        cs.cache[pluginName][key] = value

        return cs.saveToFile(pluginName)
}

// Delete 删除插件凭据键。
// 此前若 cache 中无该插件条目，会直接 return nil 不删除文件中的对应键——
// 现在先尝试从文件加载到 cache，再删除并持久化。
func (cs *CredentialStore) Delete(ctx context.Context, pluginName, key string) error {
        cs.mu.Lock()
        defer cs.mu.Unlock()

        // cache 无此插件条目时，先从文件加载（避免文件中仍有该键但内存无感知）
        if _, ok := cs.cache[pluginName]; !ok {
                creds, err := cs.loadFromFile(pluginName)
                if err != nil || creds == nil {
                        return err // 文件不存在则 err==nil 直接返回
                }
                cs.cache[pluginName] = creds
        }
        if creds, ok := cs.cache[pluginName]; ok {
                if _, exists := creds[key]; !exists {
                        return nil // 键不存在视为已删除（幂等）
                }
                delete(creds, key)
                return cs.saveToFile(pluginName)
        }
        return nil
}

// ListKeys 列出插件的所有凭据键名（不返回值）。
func (cs *CredentialStore) ListKeys(ctx context.Context, pluginName string) ([]string, error) {
        cs.mu.RLock()
        if creds, ok := cs.cache[pluginName]; ok {
                keys := make([]string, 0, len(creds))
                for k := range creds {
                        keys = append(keys, k)
                }
                cs.mu.RUnlock()
                return keys, nil
        }
        cs.mu.RUnlock()

        // 从文件加载
        creds, err := cs.loadFromFile(pluginName)
        if err != nil || creds == nil {
                return nil, err
        }
        keys := make([]string, 0, len(creds))
        for k := range creds {
                keys = append(keys, k)
        }
        return keys, nil
}

// credFilePath 返回插件凭据文件路径。
func (cs *CredentialStore) credFilePath(pluginName string) string {
        return filepath.Join(cs.dir, pluginName+".json")
}

// loadFromFile 从文件加载插件凭据。
func (cs *CredentialStore) loadFromFile(pluginName string) (map[string]string, error) {
        data, err := os.ReadFile(cs.credFilePath(pluginName))
        if err != nil {
                if os.IsNotExist(err) {
                        return nil, nil
                }
                return nil, err
        }
        // 简单的 key=value 格式（每行一对），避免引入 JSON 依赖
        creds := make(map[string]string)
        lines := splitLines(string(data))
        for _, line := range lines {
                line = trimSpace(line)
                if line == "" || line[0] == '#' {
                        continue
                }
                eq := indexByte(line, '=')
                if eq < 0 {
                        continue
                }
                key := trimSpace(line[:eq])
                value := trimSpace(line[eq+1:])
                // 反转义（与 saveToFile 对应）：把转义的 \n / \r 还原为换行/回车
                value = replaceAll(value, "\\n", "\n")
                value = replaceAll(value, "\\r", "\r")
                creds[key] = value
        }
        return creds, nil
}

// saveToFile 把插件凭据持久化到文件（key=value 格式，每行一对）。
// 按 key 升序稳定排序输出（AGENTS.md 第6条：进模型请求前缀的有序产物须显式排序）——
// 避免 map 迭代顺序随机导致同次写入产生不同文件内容（影响 diff / 备份快照）。
func (cs *CredentialStore) saveToFile(pluginName string) error {
        if err := os.MkdirAll(cs.dir, 0700); err != nil {
                return fmt.Errorf("credential store: create dir: %w", err)
        }
        creds := cs.cache[pluginName]
        keys := make([]string, 0, len(creds))
        for k := range creds {
                keys = append(keys, k)
        }
        // 升序排序
        for i := 1; i < len(keys); i++ {
                for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
                        keys[j-1], keys[j] = keys[j], keys[j-1]
                }
        }
        var buf []byte
        for _, k := range keys {
                v := creds[k]
                // 转义值中的换行/回车，避免破坏 key=value 行格式
                v = replaceAll(v, "\n", "\\n")
                v = replaceAll(v, "\r", "\\r")
                buf = append(buf, []byte(k+"="+v+"\n")...)
        }
        return os.WriteFile(cs.credFilePath(pluginName), buf, 0600)
}

// credEnvKey 把插件名 + 凭据键转为环境变量名。
// 约定：PLUGIN_NAME_KEY（全大写、连字符转下划线）。
// 例如：pluginName="llm-anthropic", key="api_key" → "LLM_ANTHROPIC_API_KEY"
func credEnvKey(pluginName, key string) string {
        s := pluginName + "_" + key
        s = replaceAll(s, "-", "_")
        s = toUpper(s)
        return s
}

// --- 简易字符串工具（避免引入 strings 包的额外依赖） ---

func splitLines(s string) []string {
        var lines []string
        start := 0
        for i := 0; i < len(s); i++ {
                if s[i] == '\n' {
                        lines = append(lines, s[start:i])
                        start = i + 1
                }
        }
        if start < len(s) {
                lines = append(lines, s[start:])
        }
        return lines
}

func trimSpace(s string) string {
        start, end := 0, len(s)
        for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
                start++
        }
        for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
                end--
        }
        return s[start:end]
}

func indexByte(s string, b byte) int {
        for i := 0; i < len(s); i++ {
                if s[i] == b {
                        return i
                }
        }
        return -1
}

func replaceAll(s, old, new string) string {
        result := ""
        for i := 0; i < len(s); {
                if i+len(old) <= len(s) && s[i:i+len(old)] == old {
                        result += new
                        i += len(old)
                } else {
                        result += string(s[i])
                        i++
                }
        }
        return result
}

func toUpper(s string) string {
        b := []byte(s)
        for i := range b {
                if b[i] >= 'a' && b[i] <= 'z' {
                        b[i] -= 'a' - 'A'
                }
        }
        return string(b)
}
