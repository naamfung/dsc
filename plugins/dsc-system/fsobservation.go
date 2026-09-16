package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"dsc/core"
	"dsc/proto"
)

// fs-observation 读前改写策略（第一实例，对齐 DSH fs-observation-policy 语义，
// 自 plugins/policy-fs-observation 迁入的核心插件混合体驻留）：
//   - str_replace/insert 前必须有本会话内的先读记录（读前改写，FS_NOT_OBSERVED 语义）
//   - create 的「不可覆盖已存在文件」语义由工具自身保证，策略无额外约束
//   - 文件自观察后被外部修改 → 拦截（sha256 新鲜度校验，FS_STALE_VERSION 语义）
//   - 读到不存在的路径记录 confirmed absent（缺失记录语义，授权后续观察与创建）
//   - 状态按会话属主隔离（per-session owner，对齐 DSH WeakMap owner——跨会话
//     不共享观察记录）；仅内存不持久：插件重启/宿主重启后无状态，会话须重新
//     读取（对齐 DSH「resumed sessions must re-read」）
//
// guardTool 受本策略守护的编辑工具（参数面：command + path）。
const guardTool = "str_replace_editor"

// observation 单文件观察记录：存在性与观察时的内容 sha256（新鲜度基准）。
type observation struct {
	present bool
	version string // present 时有效：观察时文件内容的 sha256
}

// fsObservationServer 读前改写策略服务（dsc-system 驻留，逻辑与状态全部在
// 本文件——宿主只转发工具流水线事件与执行裁决）。
type fsObservationServer struct {
	proto.UnimplementedPolicyServiceServer
	mu       sync.Mutex
	observed map[string]map[string]observation // 会话属主 → 规范化路径 → 观察记录
}

func newFsObservationServer() *fsObservationServer {
	return &fsObservationServer{observed: make(map[string]map[string]observation)}
}

// toolArgs 本插件关心的工具参数子集（guardTool 的参数面）。路径提取在插件侧
// 完成——宿主不解读任何领域语义。
type toolArgs struct {
	Command string `json:"command"`
	Path    string `json:"path"`
}

// OnEvent 实现 proto.PolicyServiceServer：按事件种类分流到读前裁决/观察记录。
func (s *fsObservationServer) OnEvent(_ context.Context, ev *proto.PolicyEvent) (*proto.PolicyDecision, error) {
	if ev.GetTool() != guardTool {
		return &proto.PolicyDecision{}, nil // 非守护工具一律放行（空裁决 = allow）
	}
	var args toolArgs
	if err := json.Unmarshal([]byte(ev.GetArgumentsJson()), &args); err != nil || args.Path == "" {
		// 参数不可解读时不设障：策略不放大工具自身的参数错误（工具会给出参数校验错误）
		return &proto.PolicyDecision{}, nil
	}
	switch ev.GetKind() {
	case kindPreExecute:
		return s.onPreExecute(ev.GetSession(), args), nil
	case kindPostExecute:
		s.onPostExecute(ev.GetSession(), ev.GetError(), args)
		return &proto.PolicyDecision{}, nil
	default:
		return &proto.PolicyDecision{}, nil
	}
}

// onPreExecute 改写前（str_replace/insert）的读前/新鲜度裁决：
//   - 未读过（unseen）→ 拦截，无论文件当前存在与否（对齐 DSH editIntent unseen ⇒
//     FS_NOT_OBSERVED；创建新文件走 create 命令）
//   - 观察为 confirmed absent → 拦截（不能编辑不存在的文件，FS_NOT_FOUND 语义）
//   - 已读但文件现已缺失 → 拦截（被外部删除，须重新确认）
//   - 已读且文件在 → sha256 新鲜度校验：不匹配即拦截（观察后被外部修改）
func (s *fsObservationServer) onPreExecute(session string, args toolArgs) *proto.PolicyDecision {
	if args.Command == "create" {
		// create 的不可覆盖语义由工具自身保证（Cannot overwrite files using
		// command 'create'）；创建新文件无需先读，策略无额外约束。
		return &proto.PolicyDecision{}
	}
	if args.Command != "str_replace" && args.Command != "insert" {
		return &proto.PolicyDecision{}
	}
	key := keyFor(session, args.Path)
	cur := statObservation(args.Path)
	s.mu.Lock()
	obs, seen := s.observed[session][key]
	s.mu.Unlock()

	if !seen {
		return deny(fmt.Sprintf("cannot modify %q: file has not been read — read the file, then retry", args.Path))
	}
	if !obs.present {
		return deny(fmt.Sprintf("cannot modify %q: file does not exist — create it with the 'create' command first", args.Path))
	}
	if !cur.present {
		return deny(fmt.Sprintf("cannot modify %q: file has been removed since it was last viewed", args.Path))
	}
	if cur.version != obs.version {
		return deny(staleMessage(args.Path))
	}
	return &proto.PolicyDecision{}
}

// onPostExecute 记录权威观察：view 无论成败都记录（存在与缺失都是观察——缺失
// 记录授权后续对同路径的观察一致性）；create/str_replace/insert 仅在成功提交后
// 记录新版本（失败未提交，不改观察状态）。
func (s *fsObservationServer) onPostExecute(session, toolErr string, args toolArgs) {
	switch args.Command {
	case "view":
		s.record(session, args.Path)
	case "create", "str_replace", "insert":
		if toolErr == "" {
			s.record(session, args.Path)
		}
	}
}

// record 记录一次权威观察（stat 判定存在性，普通文件附内容 sha256）。
// 目录不进入观察词汇（view 目录返回列表，无可编辑内容）。
func (s *fsObservationServer) record(session, path string) {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return
	}
	key := keyFor(session, path)
	obs := statObservation(path)
	s.mu.Lock()
	defer s.mu.Unlock()
	bySession := s.observed[session]
	if bySession == nil {
		bySession = make(map[string]observation)
		s.observed[session] = bySession
	}
	bySession[key] = obs
}

// statObservation 当前文件状态的权威快照：存在（普通文件且可读）时附内容 sha256；
// 缺失或不可读按未观察到场处理（present=false）。读失败（权限等）不冒充缺失语义，
// 后续改写会因 unseen 被拦截，模型重新 view 时自然获得真实错误。
func statObservation(path string) observation {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return observation{present: false}
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return observation{present: false}
	}
	sum := sha256.Sum256(content)
	return observation{present: true, version: hex.EncodeToString(sum[:])}
}

// keyFor 规范化观察键：剥离 /workspace 别名前缀；相对路径以 workspace 根为基准
// （对齐编辑器工具的解析语义——宿主经 DSC_WORKSPACE_ROOT 注入同一根）；穿透
// 符号链接/junction 到真实落点（目标不存在时解析父目录，使缺失路径也有稳定键）；
// Windows 文件系统大小写不敏感，归一小写。
func keyFor(_, path string) string {
	p := strings.TrimSpace(path)
	for _, prefix := range []string{"/workspace", `\workspace`} {
		if strings.HasPrefix(p, prefix) {
			p = strings.TrimLeft(strings.TrimPrefix(p, prefix), `/\`)
			break
		}
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(core.WorkspaceRoot, p)
	}
	p = filepath.Clean(p)
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	} else if real, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(real, filepath.Base(p))
	}
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

func deny(reason string) *proto.PolicyDecision {
	return &proto.PolicyDecision{Action: actionDeny, Reason: reason}
}

func staleMessage(path string) string {
	return fmt.Sprintf("%q has changed since it was last viewed; view it again, then retry", path)
}
