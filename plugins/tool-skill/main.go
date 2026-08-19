package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
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

// SkillScope 标识技能来源：内置（随项目自带，不可卸载）或外置（用户安装，可卸载）。
type SkillScope string

const (
	ScopeBuiltin   SkillScope = "builtin"
	ScopeInstalled SkillScope = "installed"
)

// Skill 是从 skills 目录加载的技能（playbook）。
type Skill struct {
	Name        string     // 技能名（目录名或 frontmatter name）
	Description string     // 一句话描述（显示在 system prompt 索引中）
	Body        string     // 完整 Markdown 正文
	Path        string     // SKILL.md 绝对路径
	Scope       SkillScope // 来源：builtin（内置）/ installed（外置）
}

// SkillStore 扫描并缓存内置（builtin/）与外置（installed/）技能。
type SkillStore struct {
	builtinDir   string // 内置技能目录（随项目自带，不可卸载）
	installedDir string // 外置技能目录（用户安装，可卸载）
	skills       []Skill
}

// NewSkillStore 构建并加载技能存储。
func NewSkillStore(builtinDir, installedDir string) *SkillStore {
	s := &SkillStore{builtinDir: builtinDir, installedDir: installedDir}
	s.load()
	return s
}

// load 重新扫描内置与外置技能目录。
func (s *SkillStore) load() {
	s.skills = nil
	s.loadDir(s.builtinDir, ScopeBuiltin)
	s.loadDir(s.installedDir, ScopeInstalled)
	sort.Slice(s.skills, func(i, j int) bool { return s.skills[i].Name < s.skills[j].Name })
}

// loadDir 扫描单个技能目录：支持目录布局（<dir>/SKILL.md）与扁平布局（<name>.md）。
func (s *SkillStore) loadDir(root string, scope SkillScope) {
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
					sk.Scope = scope
					s.skills = append(s.skills, sk)
				}
			}
			continue
		}
		// 扁平布局：<name>.md（根目录下直接是技能文件）
		if strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if sk, ok := parseSkillFile(filepath.Join(root, e.Name()), name); ok {
				sk.Scope = scope
				s.skills = append(s.skills, sk)
			}
		}
	}
}

func (s *SkillStore) get(name string) (Skill, bool) {
	for _, sk := range s.skills {
		if sk.Name == name {
			return sk, true
		}
	}
	return Skill{}, false
}

// removeInstalled 卸载一个外置技能（仅允许删除 installed 作用域；内置技能不可卸载）。
func (s *SkillStore) removeInstalled(name string) error {
	sk, ok := s.get(name)
	if !ok {
		return fmt.Errorf("skill not found: %s", name)
	}
	if sk.Scope != ScopeInstalled {
		return fmt.Errorf("技能 %q 是内置技能，不可卸载", name)
	}
	// 目录布局时删除整个技能目录（含资源文件）；扁平文件则只删该文件
	target := sk.Path
	if filepath.Base(filepath.Dir(sk.Path)) == sk.Name {
		target = filepath.Dir(sk.Path)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("uninstall skill %q: %w", name, err)
	}
	return nil
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
		scope := "内置"
		if sk.Scope == ScopeInstalled {
			scope = "外置"
		}
		b.WriteString(fmt.Sprintf("- %s [%s] — %s\n", sk.Name, scope, desc))
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

// ---------- install_skill：按路径自动安装技能包 ----------

const (
	maxSkillScanDepth = 3   // 平级遍历目录的最大深度
	maxSkillScanCount = 200 // 一次扫描最多发现的技能数
	maxSkillCopyBytes = 20 << 20 // 单技能包拷贝上限（20MB）
)

// SkillCandidate 是从源路径发现、待安装的技能包。
type SkillCandidate struct {
	Name       string // 技能名（frontmatter name 或目录名）
	SourceDir  string // 目录布局技能：整个技能包目录
	SourceFile string // 扁平布局技能：单个 .md 文件
	DirLayout  bool   // 是否为目录布局（<name>/SKILL.md）
}

// InstallSkillTool 根据用户给出的路径查找技能包并安装为本机外置技能。
// 用户日常可喊模型「帮我安装某 SKILL，SKILL 路径」，模型调用本工具即可完成安装。
// 安装位置为 skills/installed 目录（外置技能，可卸载），区别于 skills/builtin 内置技能。
type InstallSkillTool struct {
	store        *SkillStore
	installedDir string // 外置技能安装目录（默认 <skillsDir>/installed）
}

func (t *InstallSkillTool) Name() string { return "install_skill" }

func (t *InstallSkillTool) Description() string {
	return "根据用户提供的路径安装技能包：平级遍历该目录自动找到 SKILL.md（或带 name/description frontmatter 的 <name>.md），校验后拷贝到本机 skills 目录，重启后出现在 /skills 命令与技能索引中。参数 path 为技能包路径（单个文件、技能包目录或包含多个技能包的父目录）。"
}

func (t *InstallSkillTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"技能包路径：指向单个 SKILL.md 文件、技能包目录（内含 SKILL.md），或包含多个技能包的父目录"}},"required":["path"]}`)
}

func (t *InstallSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("skill source not found: %w", err)
	}

	var cands []SkillCandidate
	if !info.IsDir() {
		c, ok := candidateFromFile(abs)
		if !ok {
			return "", fmt.Errorf("no valid skill at %s：需要带 name/description frontmatter 的 SKILL.md 或 <name>.md", abs)
		}
		cands = append(cands, c)
	} else {
		cands = scanSkillDir(abs)
		if len(cands) == 0 {
			return "", fmt.Errorf("no skill found under %s（未找到 SKILL.md 或带 frontmatter 的 <name>.md）", abs)
		}
	}

	installed := make([]string, 0, len(cands))
	for _, c := range cands {
		if err := t.installCandidate(c); err != nil {
			return "", err
		}
		installed = append(installed, c.Name)
	}
	// 安装后刷新技能存储，read_skill 与索引立即可用（system prompt 索引在重启后重建）
	t.store.load()

	res, _ := json.Marshal(map[string]any{
		"ok":          true,
		"installed":   installed,
		"installedDir": t.installedDir,
		"note":        "已安装为外置技能，重启后出现在 /skills 命令与 system prompt 技能索引中；当前会话可用 read_skill 直接读取，可用 uninstall_skill 卸载。",
	})
	return string(res), nil
}

// candidateFromFile 从单个文件识别技能包：SKILL.md（名字取父目录名）或 <name>.md（名字取文件名）。
func candidateFromFile(path string) (SkillCandidate, bool) {
	base := filepath.Base(path)
	var name string
	if strings.EqualFold(base, "SKILL.md") {
		name = filepath.Base(filepath.Dir(path))
	} else if strings.HasSuffix(strings.ToLower(base), ".md") {
		name = strings.TrimSuffix(base, filepath.Ext(base))
	} else {
		return SkillCandidate{}, false
	}
	sk, ok := parseSkillFile(path, name)
	if !ok || strings.TrimSpace(sk.Description) == "" {
		return SkillCandidate{}, false // 安装要求 description，保证索引可用
	}
	return SkillCandidate{Name: sk.Name, SourceFile: path}, true
}

// scanSkillDir 平级遍历目录发现技能包（深度受 maxSkillScanDepth 限制）：
// 目录布局 <dir>/SKILL.md 视为一个自包含技能包（停止下钻，内部 .md 不当作独立技能）；
// 其余可达的 <name>.md 视为扁平技能。
func scanSkillDir(root string) []SkillCandidate {
	var out []SkillCandidate
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 忽略无法访问的条目
		}
		if path == root {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		depth := len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
		if d.IsDir() {
			if depth > maxSkillScanDepth {
				return filepath.SkipDir
			}
			if strings.EqualFold(d.Name(), ".git") {
				return filepath.SkipDir
			}
			skillFile := filepath.Join(path, "SKILL.md")
			if info, statErr := os.Stat(skillFile); statErr == nil && !info.IsDir() {
				if sk, ok := parseSkillFile(skillFile, d.Name()); ok && strings.TrimSpace(sk.Description) != "" {
					out = append(out, SkillCandidate{Name: sk.Name, SourceDir: path, DirLayout: true})
				}
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= maxSkillScanCount {
			return nil
		}
		if !d.Type().IsRegular() || !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
			return nil
		}
		if strings.EqualFold(d.Name(), "SKILL.md") {
			return nil // 根级 SKILL.md 由目录布局逻辑处理
		}
		if sk, ok := parseSkillFile(path, strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))); ok && strings.TrimSpace(sk.Description) != "" {
			out = append(out, SkillCandidate{Name: sk.Name, SourceFile: path})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// installCandidate 将技能包拷贝到外置技能目录，统一为目录布局 <name>/SKILL.md。
func (t *InstallSkillTool) installCandidate(c SkillCandidate) error {
	destDir := filepath.Join(t.installedDir, c.Name)
	if c.DirLayout {
		if err := copyDir(c.SourceDir, destDir); err != nil {
			return fmt.Errorf("install skill %q: %w", c.Name, err)
		}
	} else {
		if err := copyFile(c.SourceFile, filepath.Join(destDir, "SKILL.md")); err != nil {
			return fmt.Errorf("install skill %q: %w", c.Name, err)
		}
	}
	return nil
}

// copyDir 递归拷贝技能包目录（跳过 .git），保留技能引用的资源文件。
func copyDir(src, dst string) error {
	var copied int64
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if strings.EqualFold(d.Name(), ".git") {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		copied += info.Size()
		if copied > maxSkillCopyBytes {
			return fmt.Errorf("skill directory exceeds %d bytes", maxSkillCopyBytes)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// UninstallSkillTool 卸载（删除）用户安装的外置技能；内置技能（skills/builtin）不可卸载。
type UninstallSkillTool struct {
	store *SkillStore
}

func (t *UninstallSkillTool) Name() string { return "uninstall_skill" }

func (t *UninstallSkillTool) Description() string {
	return "卸载（删除）一个用户安装的外置技能：从本机 skills/installed 目录删除该技能包及其资源文件，重启后从 /skills 命令与技能索引中移除。内置技能（skills/builtin，如 git-commit）不可卸载。参数 name 为技能名。"
}

func (t *UninstallSkillTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"name":{"type":"string","description":"要卸载的技能名（必须是外置技能，内置技能不可卸载）"}},"required":["name"]}`)
}

func (t *UninstallSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	name := strings.TrimSpace(p.Name)
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if err := t.store.removeInstalled(name); err != nil {
		return "", err
	}
	// 卸载后刷新技能存储，read_skill 与索引立即生效
	t.store.load()

	res, _ := json.Marshal(map[string]any{
		"ok":          true,
		"uninstalled": name,
		"note":        "技能已卸载，重启后从 /skills 与 system prompt 技能索引中移除。",
	})
	return string(res), nil
}

// ToolServiceServer 工具服務服務端實現
type ToolServiceServer struct {
	proto.UnimplementedToolServiceServer
	store         *SkillStore
	readTool      *ReadSkillTool
	installTool   *InstallSkillTool
	uninstallTool *UninstallSkillTool
}

func (s *ToolServiceServer) ExecuteTool(ctx context.Context, req *proto.ExecuteToolRequest) (*proto.ExecuteToolResponse, error) {
	switch req.ToolName {
	case s.readTool.Name():
		res, err := s.readTool.Execute(ctx, json.RawMessage(req.ArgumentsJson))
		if err != nil {
			return &proto.ExecuteToolResponse{Error: err.Error()}, nil
		}
		return &proto.ExecuteToolResponse{Content: res}, nil
	case s.installTool.Name():
		res, err := s.installTool.Execute(ctx, json.RawMessage(req.ArgumentsJson))
		if err != nil {
			return &proto.ExecuteToolResponse{Error: err.Error()}, nil
		}
		return &proto.ExecuteToolResponse{Content: res}, nil
	case s.uninstallTool.Name():
		res, err := s.uninstallTool.Execute(ctx, json.RawMessage(req.ArgumentsJson))
		if err != nil {
			return &proto.ExecuteToolResponse{Error: err.Error()}, nil
		}
		return &proto.ExecuteToolResponse{Content: res}, nil
	}
	return &proto.ExecuteToolResponse{Error: "tool not found"}, nil
}

func (s *ToolServiceServer) ListTools(ctx context.Context, req *proto.ListToolsRequest) (*proto.ListToolsResponse, error) {
	return &proto.ListToolsResponse{Tools: []*proto.Tool{
		{
			Name:           s.readTool.Name(),
			Description:    s.readTool.Description(),
			ParametersJson: string(s.readTool.ParametersSchema()),
		},
		{
			Name:           s.installTool.Name(),
			Description:    s.installTool.Description(),
			ParametersJson: string(s.installTool.ParametersSchema()),
		},
		{
			Name:           s.uninstallTool.Name(),
			Description:    s.uninstallTool.Description(),
			ParametersJson: string(s.uninstallTool.ParametersSchema()),
		},
	}}, nil
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
	builtinDir := filepath.Join(skillsDir, "builtin")
	installedDir := filepath.Join(skillsDir, "installed")
	store := NewSkillStore(builtinDir, installedDir)

	readTool := &ReadSkillTool{store: store}
	installTool := &InstallSkillTool{store: store, installedDir: installedDir}
	uninstallTool := &UninstallSkillTool{store: store}
	toolServer := &ToolServiceServer{store: store, readTool: readTool, installTool: installTool, uninstallTool: uninstallTool}
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
