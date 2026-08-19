package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dsc/plugin"
	"dsc/proto"
	"dsc/proto/metadata"
	goplugin "github.com/hashicorp/go-plugin"
	"gopkg.in/yaml.v3"
)

// Skill 是从 skills 目录加载的技能（playbook）。
type Skill struct {
	Name        string // 技能名（目录名或 frontmatter name）
	Description string // 一句话描述（显示在 system prompt 索引中）
	Body        string // 完整 Markdown 正文
	Path        string // SKILL.md 绝对路径
}

// SkillStore 扫描并缓存 skills 目录中的技能。
type SkillStore struct {
	skills []Skill
}

// load 扫描技能目录：支持目录布局（<dir>/SKILL.md）与扁平布局（<name>.md）。
func (s *SkillStore) load(root string) {
	s.skills = nil
	entries, err := os.ReadDir(root)
	if err != nil {
		return // 目录不存在视为无技能
	}
	for _, e := range entries {
		if e.IsDir() {
			// 目录布局：<dir>/SKILL.md
			skillFile := filepath.Join(root, e.Name(), "SKILL.md")
			if info, err := os.Stat(skillFile); err == nil && !info.IsDir() {
				if sk, ok := parseSkillFile(skillFile, e.Name()); ok {
					s.skills = append(s.skills, sk)
				}
			}
			continue
		}
		// 扁平布局：<name>.md（根目录下直接是技能文件）
		if strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if sk, ok := parseSkillFile(filepath.Join(root, e.Name()), name); ok {
				s.skills = append(s.skills, sk)
			}
		}
	}
	sort.Slice(s.skills, func(i, j int) bool { return s.skills[i].Name < s.skills[j].Name })
}

func (s *SkillStore) get(name string) (Skill, bool) {
	for _, sk := range s.skills {
		if sk.Name == name {
			return sk, true
		}
	}
	return Skill{}, false
}

// parseSkillFile 解析 SKILL.md：可选 YAML frontmatter（name/description）+ Markdown 正文。
func parseSkillFile(path, fallbackName string) (Skill, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, false
	}
	content := string(data)
	sk := Skill{Name: fallbackName, Path: path}

	if strings.HasPrefix(content, "---") {
		rest := strings.TrimPrefix(content, "---")
		if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if end := strings.Index(rest, "\n---"); end > 0 {
			fmRaw := rest[:end]
			body := rest[end+len("\n---"):]
			var fm struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if err := yaml.Unmarshal([]byte(fmRaw), &fm); err == nil {
				if strings.TrimSpace(fm.Name) != "" {
					sk.Name = strings.TrimSpace(fm.Name)
				}
				sk.Description = strings.TrimSpace(fm.Description)
			}
			content = strings.TrimLeft(body, "\n")
		}
	}
	sk.Body = strings.TrimSpace(content)
	if sk.Body == "" {
		return Skill{}, false
	}
	return sk, true
}

// indexBlock 生成注入 system prompt 的技能索引（只含名字+描述，正文按需 read_skill 加载），防止撑爆上下文。
func (s *SkillStore) indexBlock() string {
	if len(s.skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("# Skills — 可调用的技能\n\n")
	b.WriteString("开始非平凡任务前先浏览此索引：若某技能与本任务相关，先调用 read_skill 读取其正文，再按其指示执行。\n")
	b.WriteString("调用方式：read_skill({ \"name\": \"<技能名>\" })\n\n")
	for _, sk := range s.skills {
		desc := strings.TrimSpace(strings.ReplaceAll(sk.Description, "\n", " "))
		if desc == "" {
			desc = "(no description)"
		}
		b.WriteString(fmt.Sprintf("- %s — %s\n", sk.Name, desc))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ReadSkillTool 读取技能正文工具：模型调用后把正文当工具结果读入上下文（inline 方式执行技能）。
type ReadSkillTool struct {
	store *SkillStore
}

func (t *ReadSkillTool) Name() string        { return "read_skill" }
func (t *ReadSkillTool) Description() string { return "读取一个技能（skill）的完整正文，按其指示执行；参数 name 为技能名。" }
func (t *ReadSkillTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"要读取的技能名"}},"required":["name"]}`)
}

func (t *ReadSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("name is required")
	}
	sk, ok := t.store.get(p.Name)
	if !ok {
		return "", fmt.Errorf("skill not found: %s", p.Name)
	}
	return sk.Body, nil
}

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	store    *SkillStore
	readTool *ReadSkillTool
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	if req.ToolName == s.readTool.Name() {
		res, err := s.readTool.Execute(ctx, json.RawMessage(req.ArgumentsJson))
		if err != nil {
			return &proto.ExecuteToolResponse{Error: err.Error()}, nil
		}
		return &proto.ExecuteToolResponse{Content: res}, nil
	}
	return &proto.ExecuteToolResponse{Error: "tool not found"}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{Tools: []*proto.Tool{{
		Name:           s.readTool.Name(),
		Description:    s.readTool.Description(),
		ParametersJson: string(s.readTool.ParametersSchema()),
	}}}, nil
}

// ListContext 返回技能索引块，宿主将其拼接到 agent 的 system prompt。
func (s *ToolServiceServer) ListContext(ctx context.Context, req *proto.ListContextRequest) (*proto.ListContextResponse, error) {
	return &proto.ListContextResponse{Content: s.store.indexBlock()}, nil
}

// MetadataServer 元數據服務服務端實現
type MetadataServer struct {
	metadata.UnimplementedPluginMetadataServer
}

func (m *MetadataServer) GetInfo(ctx context.Context, _ *metadata.Empty) (*metadata.PluginInfo, error) {
	return &metadata.PluginInfo{
		Type:       "tool",
		Name:       "skill",
		Version:    "1.0.0",
		ApiVersion: "1.0",
	}, nil
}

func main() {
	// 技能目录由宿主通过环境变量传入（未设置时默认 ./skills）
	skillsDir := os.Getenv("DSC_SKILLS_DIR")
	if skillsDir == "" {
		skillsDir = "./skills"
	}
	store := &SkillStore{}
	store.load(skillsDir)

	readTool := &ReadSkillTool{store: store}
	toolServer := &ToolServiceServer{store: store, readTool: readTool}
	metadataServer := &MetadataServer{}

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: plugin.Handshake,
		Plugins: map[string]goplugin.Plugin{
			"tool": &ToolMetadataGRPCPlugin{
				ToolImpl:     toolServer,
				MetadataImpl: metadataServer,
			},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
	})
}
