package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestCronToolsRegistered 验证宿主把 cron_add/cron_list/cron_remove/cron_set_enabled
// 四个工具注册到 ToolRegistry，使模型在评测中可真实调用 DSC 的 CRON 机制。
func TestCronToolsRegistered(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	var names []string
	for _, td := range m.cronTools() {
		names = append(names, td.Name())
	}
	for _, want := range []string{"cron_add", "cron_list", "cron_remove", "cron_set_enabled"} {
		if !containsString(names, want) {
			t.Errorf("cronTools 应含 %s，got %v", want, names)
		}
	}
	// 校验全部工具均经 m.toolRegistry.Register 注册（模型可调用）
	for _, want := range []string{"cron_add", "cron_list", "cron_remove", "cron_set_enabled"} {
		if _, ok := m.toolRegistry.Get(want); !ok {
			t.Errorf("ToolRegistry 应含 %s（模型可见），未找到", want)
		}
	}
}

// TestCronToolsSchemasValid 工具的参数 schema 必须是合法 JSON。
func TestCronToolsSchemasValid(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	for _, td := range m.cronTools() {
		if err := json.Unmarshal(td.ParametersSchema(), &map[string]any{}); err != nil {
			t.Errorf("%s: schema 非法 JSON: %v", td.Name(), err)
		}
	}
}

// TestCronAddRejectsBadInput 校验 cron_add 工具拒绝坏输入：空名、含特殊字符的名、
// 空 cron 表达式、空 prompt。这些不应触达宿主侧的 AddCronJob。
func TestCronAddRejectsBadInput(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	tool := &cronAddTool{m: m}
	bad := []string{
		`{}`,
		`{"name":"", "cron":"0 8 * * *", "prompt":"x"}`,
		`{"name":"bad name", "cron":"0 8 * * *", "prompt":"x"}`, // 名含空格
		`{"name":"a/b", "cron":"0 8 * * *", "prompt":"x"}`,      // 名含斜杠
		`{"name":"ok", "cron":"", "prompt":"x"}`,                // 空 cron
		`{"name":"ok", "cron":"0 8 * * *", "prompt":""}`,        // 空 prompt
	}
	for _, args := range bad {
		_, err := tool.Execute(context.Background(), json.RawMessage(args))
		if err == nil {
			t.Errorf("cron_add 应拒绝坏输入 %s", args)
		}
	}
}

// TestCronAddRequiresScheduler cron_add 在调度器未启动时应返回错误，而非静默成功。
func TestCronAddRequiresScheduler(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	tool := &cronAddTool{m: m}
	_, err := tool.Execute(context.Background(), json.RawMessage(`{
                "name":"test","cron":"0 8 * * *","prompt":"x"
        }`))
	if err == nil {
		t.Fatal("cron_add 应在调度器未启动时报错，而非静默成功")
	}
	if !strings.Contains(err.Error(), "scheduler") {
		t.Errorf("错误信息应提及 scheduler，got %v", err)
	}
}

// TestCronListEmptyWhenNoScheduler cron_list 在调度器未启动时返回空列表（不报错）。
func TestCronListEmptyWhenNoScheduler(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	tool := &cronListTool{m: m}
	out, err := tool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cron_list 不应在无调度器时报错: %v", err)
	}
	var r struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("cron_list 输出非合法 JSON: %v", err)
	}
	if r.Count != 0 {
		t.Errorf("无调度器时 cron_list 应返回空，got count=%d", r.Count)
	}
}

// TestCronApprovalReasons cron_add / cron_remove / cron_set_enabled 应声明 ApprovalReason
// （变更持久化任务集，属高影响变更，须经用户审批）。cron_list 是只读工具，不需要审批。
func TestCronApprovalReasons(t *testing.T) {
	m := NewManager(&ManagerConfig{})
	// 变更类工具：应实现 ApprovalRequester
	mutating := []string{"cron_add", "cron_remove", "cron_set_enabled"}
	for _, want := range mutating {
		td, ok := m.toolRegistry.Get(want)
		if !ok {
			t.Fatalf("ToolRegistry 未注册 %s", want)
		}
		ar, ok := td.(ApprovalRequester)
		if !ok {
			t.Errorf("%s 应实现 ApprovalRequester", want)
			continue
		}
		reason := ar.ApprovalReason("{}")
		if reason == "" {
			t.Errorf("%s: ApprovalReason 不应为空", want)
		}
	}
	// 只读工具 cron_list：不应实现 ApprovalRequester（无需审批）
	if td, ok := m.toolRegistry.Get("cron_list"); ok {
		if _, isReq := td.(ApprovalRequester); isReq {
			t.Error("cron_list 是只读工具，不应实现 ApprovalRequester")
		}
	}
}

// TestCronAddSuccessAndList 端到端：启动 cron 调度器后，cron_add 添加任务，
// cron_list 应能列出该任务（含正确字段）。
func TestCronAddSuccessAndList(t *testing.T) {
	// 创建临时 ExecDir 让 cron.json 落盘到测试隔离目录
	dir := t.TempDir()
	m := NewManager(&ManagerConfig{ExecDir: dir})
	if err := m.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}
	defer m.StopCron()

	addTool := &cronAddTool{m: m}
	out, err := addTool.Execute(context.Background(), json.RawMessage(`{
                "name":"test-job","cron":"0 8 * * *","prompt":"greeting"
        }`))
	if err != nil {
		t.Fatalf("cron_add: %v", err)
	}
	var add struct {
		OK      bool   `json:"ok"`
		ID      string `json:"id"`
		Name    string `json:"name"`
		Cron    string `json:"cron"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(out), &add); err != nil {
		t.Fatalf("cron_add 输出非 JSON: %v", err)
	}
	if !add.OK || add.ID == "" || !strings.HasPrefix(add.ID, "cron-") {
		t.Errorf("cron_add 返回异常: %+v", add)
	}
	if add.Name != "test-job" || add.Cron != "0 8 * * *" || !add.Enabled {
		t.Errorf("cron_add 字段保真异常: %+v", add)
	}

	listTool := &cronListTool{m: m}
	listOut, err := listTool.Execute(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("cron_list: %v", err)
	}
	var list struct {
		Jobs []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Cron    string `json:"cron"`
			Enabled bool   `json:"enabled"`
		} `json:"jobs"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(listOut), &list); err != nil {
		t.Fatalf("cron_list 输出非 JSON: %v", err)
	}
	if list.Count < 1 {
		t.Fatalf("cron_list 应至少含 1 个任务，got %d", list.Count)
	}
	var found bool
	for _, j := range list.Jobs {
		if j.ID == add.ID {
			found = true
			if j.Name != "test-job" || j.Cron != "0 8 * * *" || !j.Enabled {
				t.Errorf("cron_list 字段保真异常: %+v", j)
			}
		}
	}
	if !found {
		t.Errorf("cron_list 未列出 cron_add 刚添加的任务 %s", add.ID)
	}
}

// TestCronSetEnabledAndRemove 端到端：cron_set_enabled 把任务禁用后 cron_list 应反映
// enabled=false；cron_remove 删除后 cron_list 不再含该 id。
func TestCronSetEnabledAndRemove(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(&ManagerConfig{ExecDir: dir})
	if err := m.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}
	defer m.StopCron()

	// 添加任务
	addTool := &cronAddTool{m: m}
	out, _ := addTool.Execute(context.Background(), json.RawMessage(`{
                "name":"rt-job","cron":"*/15 * * * *","prompt":"recurring"
        }`))
	var add struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(out), &add)
	if add.ID == "" {
		t.Fatal("cron_add 未返回 id")
	}

	// 禁用
	setTool := &cronSetEnabledTool{m: m}
	if _, err := setTool.Execute(context.Background(), json.RawMessage(fmt.Sprintf(
		`{"id":%q,"enabled":false}`, add.ID))); err != nil {
		t.Fatalf("cron_set_enabled: %v", err)
	}

	// 验证 cron_list 反映 disabled
	listTool := &cronListTool{m: m}
	listOut, _ := listTool.Execute(context.Background(), json.RawMessage(`{}`))
	var list struct {
		Jobs []struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"jobs"`
	}
	_ = json.Unmarshal([]byte(listOut), &list)
	var stillExists bool
	for _, j := range list.Jobs {
		if j.ID == add.ID {
			stillExists = true
			if j.Enabled {
				t.Errorf("cron_set_enabled(false) 后 cron_list 应反映 enabled=false")
			}
		}
	}
	if !stillExists {
		t.Error("cron_set_enabled 不应删除任务，cron_list 仍应含该 id")
	}

	// 删除
	removeTool := &cronRemoveTool{m: m}
	if _, err := removeTool.Execute(context.Background(), json.RawMessage(fmt.Sprintf(
		`{"id":%q}`, add.ID))); err != nil {
		t.Fatalf("cron_remove: %v", err)
	}

	// 验证 cron_list 不再含该 id
	listOut2, _ := listTool.Execute(context.Background(), json.RawMessage(`{}`))
	var list2 struct {
		Jobs []struct {
			ID string `json:"id"`
		} `json:"jobs"`
	}
	_ = json.Unmarshal([]byte(listOut2), &list2)
	for _, j := range list2.Jobs {
		if j.ID == add.ID {
			t.Errorf("cron_remove 后 cron_list 不应再含 id %s", add.ID)
		}
	}
}

// TestCronAddInvalidCronExpression cron_add 在 cron 表达式非法时拒绝（不创建任务）。
func TestCronAddInvalidCronExpression(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(&ManagerConfig{ExecDir: dir})
	if err := m.StartCron(); err != nil {
		t.Fatalf("StartCron: %v", err)
	}
	defer m.StopCron()

	addTool := &cronAddTool{m: m}
	_, err := addTool.Execute(context.Background(), json.RawMessage(`{
                "name":"bad","cron":"not a real cron","prompt":"x"
        }`))
	if err == nil {
		t.Error("cron_add 应拒绝非法 cron 表达式")
	}
}
