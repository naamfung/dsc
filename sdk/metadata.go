package dsc

import (
	"context"
	"strings"

	"dsc/proto/metadata"
)

// metadataServer 插件元数据服务：宿主按配置加载时校验类型/版本。
type metadataServer struct {
	metadata.UnimplementedPluginMetadataServer
	cfg Config
}

// requiresCapabilityKeyPrefix 是声明式依赖在 PluginInfo.Capabilities 中的编码前缀。
// 复用现有 capabilities map（避免改动 proto）：
//   - 键形如 "requires/llm/supports_images" 或 "requires/tool/cron"
//   - 值固定为 "true"
//
// 宿主扫描 PluginInfo.Capabilities 时见到此前缀即知本插件声明了对某类型插件、某能力
// 的依赖；扫描其余已加载插件的 capabilities（普通能力键，无前缀）找到首个匹配者并
// 建立依赖关系（存于宿主运行时态 m.resolvedDeps）。普通能力键不带前缀，表示本插件
// **提供**了该能力——其他插件经 Requires 声明对此能力的依赖时，宿主据此匹配。
// 此编码对齐 DSH/Cordis 的 provide + inject 模型——插件按能力边界声明依赖，不按插件名引用。
const requiresCapabilityKeyPrefix = "requires/"

func (s *metadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	// 合并 Provides（普通能力键）与 Requires（requires/<type>/<cap> 前缀键）
	caps := make(map[string]string)
	// 1. Provides：普通能力键（如 "filesystem": "true"），表示本插件提供该能力
	for k, v := range s.cfg.Provides {
		if k == "" || strings.HasPrefix(k, requiresCapabilityKeyPrefix) {
			continue // 跳过空键与误用 requires/ 前缀的键
		}
		caps[k] = v
	}
	// 2. Requires：requires/<type>/<cap> 前缀键，表示本插件依赖某能力
	for k, v := range EncodeRequires(s.cfg) {
		caps[k] = v
	}
	return &metadata.PluginInfo{
		Type:         string(s.cfg.Type),
		Name:         s.cfg.Name,
		Version:      s.cfg.Version,
		ApiVersion:   s.cfg.APIVersion,
		Capabilities: caps,
	}, nil
}

// parseRequiresCapability 解析一个 capabilities 键，若是 requires/<type>/<cap> 形式
// 则返回 (type, cap, true)，否则返回 ("", "", false)。
//
// 暴露给宿主侧（host）使用：宿主侧读 PluginInfo.Capabilities 时调用此函数提取声明式
// 依赖。SDK 内部声明与宿主侧解析共享同一编码约定（见 requiresCapabilityKeyPrefix）。
func parseRequiresCapability(key string) (reqType, capability string, ok bool) {
	if len(key) <= len(requiresCapabilityKeyPrefix) {
		return "", "", false
	}
	if key[:len(requiresCapabilityKeyPrefix)] != requiresCapabilityKeyPrefix {
		return "", "", false
	}
	rest := key[len(requiresCapabilityKeyPrefix):]
	// 形如 "llm/supports_images" 或 "tool/cron"：按第一个 "/" 拆分
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			reqType = rest[:i]
			capability = rest[i+1:]
			if reqType != "" && capability != "" && !containsSlash(capability) {
				return reqType, capability, true
			}
			return "", "", false
		}
	}
	return "", "", false
}

// containsSlash 报告 s 是否含 '/'（capability 名不允许跨层级）。
func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}

// EncodeRequires 把 Config.Requires 编码为 capabilities map 条目，便于本包测试
// 与外部检查 SDK 自身声明（仍以 metadataServer.GetInfo 为准）。
func EncodeRequires(cfg Config) map[string]string {
	caps := make(map[string]string, len(cfg.Requires))
	for _, r := range cfg.Requires {
		if r.Type == "" || r.Capability == "" {
			continue
		}
		caps[requiresCapabilityKeyPrefix+r.Type+"/"+r.Capability] = "true"
	}
	return caps
}
