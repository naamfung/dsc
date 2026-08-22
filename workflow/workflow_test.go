package workflow

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRunner 记录调用并返回预设结果/错误。
type fakeRunner struct {
	mu    sync.Mutex
	calls []string
	resps map[string]string
	errs  map[string]error
}

func (f *fakeRunner) RunAgent(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, prompt)
	if err := f.errs[prompt]; err != nil {
		return "", err
	}
	return f.resps[prompt], nil
}

// recSink 记录观测事件。
type recSink struct {
	mu     sync.Mutex
	starts int
	phases []string
	logs   []string
	agents []string
	ends   int
}

func (r *recSink) OnStart(string, Meta) { r.mu.Lock(); r.starts++; r.mu.Unlock() }
func (r *recSink) OnPhase(_ string, t string) {
	r.mu.Lock()
	r.phases = append(r.phases, t)
	r.mu.Unlock()
}
func (r *recSink) OnLog(_ string, m string) { r.mu.Lock(); r.logs = append(r.logs, m); r.mu.Unlock() }
func (r *recSink) OnAgentStart(_ string, seq int, l string) {
	r.mu.Lock()
	r.agents = append(r.agents, "start:"+l)
	r.mu.Unlock()
}
func (r *recSink) OnAgentEnd(_ string, seq int, o string) {
	r.mu.Lock()
	r.agents = append(r.agents, "end:"+o)
	r.mu.Unlock()
}
func (r *recSink) OnEnd(string, Result) { r.mu.Lock(); r.ends++; r.mu.Unlock() }

func run(t *testing.T, req StartRequest) Result {
	t.Helper()
	run, err := Start(context.Background(), req)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case r := <-run.Result:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("run did not settle within 3s")
		return Result{}
	}
}

func TestValidateMeta(t *testing.T) {
	if err := ValidateMeta(Meta{Name: "daily-report", Description: "x"}); err != nil {
		t.Fatalf("valid meta rejected: %v", err)
	}
	if err := ValidateMeta(Meta{Name: "", Description: "x"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := ValidateMeta(Meta{Name: "Bad Name", Description: "x"}); err == nil {
		t.Fatal("non-kebab name should fail")
	}
	if err := ValidateMeta(Meta{Name: "ok", Description: ""}); err == nil {
		t.Fatal("empty description should fail")
	}
	if err := ValidateMeta(Meta{Name: "ok", Description: "x",
		Phases: []Phase{{Title: "a"}, {Title: "a"}}}); err == nil {
		t.Fatal("duplicate phase should fail")
	}
}

func TestStartInvalidScript(t *testing.T) {
	req := StartRequest{Meta: Meta{Name: "x", Description: "y"}, Script: "return (", Runner: &fakeRunner{}}
	_, err := Start(context.Background(), req)
	re, ok := err.(*RunError)
	if !ok || re.Code != ErrScriptParse {
		t.Fatalf("syntax error should be SCRIPT_PARSE, got %v", err)
	}
}

func TestRunCompleted(t *testing.T) {
	r := run(t, StartRequest{
		Meta:   Meta{Name: "simple", Description: "returns an object"},
		Script: `return {ok: true, n: 3};`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop reason = %q, err = %s", r.StopReason, r.Error)
	}
	v, _ := r.Value.(map[string]any)
	if v == nil || v["ok"] != true || v["n"] != int64(3) {
		t.Fatalf("value = %+v", r.Value)
	}
	if r.AgentsStarted != 0 {
		t.Fatalf("agentsStarted = %d", r.AgentsStarted)
	}
}

func TestRunAgentHook(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"a": "result-a", "b": "result-b"}}
	sink := &recSink{}
	r := run(t, StartRequest{
		Meta:   Meta{Name: "fanout", Description: "two agents"},
		Script: `const x = await agent("a"); const y = await agent("b"); return [x, y];`,
		Runner: fr,
		Events: sink,
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop reason = %q, err = %s", r.StopReason, r.Error)
	}
	arr, _ := r.Value.([]any)
	if len(arr) != 2 || arr[0] != "result-a" || arr[1] != "result-b" {
		t.Fatalf("value = %+v", r.Value)
	}
	if r.AgentsStarted != 2 || len(fr.calls) != 2 {
		t.Fatalf("agents = %d, calls = %v", r.AgentsStarted, fr.calls)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.starts != 1 || sink.ends != 1 || len(sink.agents) != 4 {
		t.Fatalf("sink events: starts=%d ends=%d agents=%v", sink.starts, sink.ends, sink.agents)
	}
}

func TestRunAgentChildFailureReturnsNull(t *testing.T) {
	fr := &fakeRunner{errs: map[string]error{"boom": context.Canceled}}
	r := run(t, StartRequest{
		Meta:   Meta{Name: "fail", Description: "child fails"},
		Script: `const x = await agent("boom"); return x === null;`,
		Runner: fr,
	})
	if r.StopReason != StopCompleted || r.Value != true {
		t.Fatalf("child failure should surface as null, got %+v", r)
	}
}

func TestRunAgentCap(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"a": "x"}}
	r := run(t, StartRequest{
		Meta:           Meta{Name: "cap", Description: "cap test"},
		Script:         `await agent("a"); await agent("a"); return 1;`,
		Runner:         fr,
		MaxTotalAgents: 1,
	})
	if r.StopReason != StopError || !strings.Contains(r.Error, ErrAgentCap) {
		t.Fatalf("cap should fail with AGENT_CAP, got %+v", r)
	}
}

func TestRunPhaseValidation(t *testing.T) {
	fr := &fakeRunner{}
	// 声明了 phases 时，未声明的标题报错
	r := run(t, StartRequest{
		Meta:   Meta{Name: "ph", Description: "x", Phases: []Phase{{Title: "build"}}},
		Script: `phase("nope"); return 1;`,
		Runner: fr,
	})
	if r.StopReason != StopError || !strings.Contains(r.Error, "not declared") {
		t.Fatalf("undeclared phase should fail, got %+v", r)
	}
	// 声明内的标题可用
	sink := &recSink{}
	r = run(t, StartRequest{
		Meta:   Meta{Name: "ph2", Description: "x", Phases: []Phase{{Title: "build"}}},
		Script: `phase("build"); log("hello"); return 1;`,
		Runner: fr,
		Events: sink,
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("declared phase should pass, got %+v", r)
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.phases) != 1 || sink.phases[0] != "build" || len(sink.logs) != 1 || sink.logs[0] != "hello" {
		t.Fatalf("events: phases=%v logs=%v", sink.phases, sink.logs)
	}
}

func TestRunArgs(t *testing.T) {
	r := run(t, StartRequest{
		Meta:   Meta{Name: "args", Description: "x"},
		Script: `return {got: args.query};`,
		Args:   map[string]any{"query": "dsc"},
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("got %+v", r)
	}
	v, _ := r.Value.(map[string]any)
	if v == nil || v["got"] != "dsc" {
		t.Fatalf("value = %+v", r.Value)
	}
}

func TestRunResultUnserializable(t *testing.T) {
	r := run(t, StartRequest{
		Meta:   Meta{Name: "bad", Description: "x"},
		Script: `return function(){};`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopError || !strings.Contains(r.Error, ErrResultUnserializable) {
		t.Fatalf("function return should be unserializable, got %+v", r)
	}
}

func TestRunCancelled(t *testing.T) {
	fr := &fakeRunner{}
	run, err := Start(context.Background(), StartRequest{
		Meta:    Meta{Name: "hang", Description: "x"},
		Script:  `while (Date.now() < 1e15) {} return 1;`,
		Runner:  fr,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	run.Cancel()
	select {
	case r := <-run.Result:
		if r.StopReason != StopCancelled {
			t.Fatalf("cancel should settle cancelled, got %+v", r)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled run did not settle")
	}
}

// slowRunner 每个子 agent 固定延迟，用于验证并发扇出的耗时。
type slowRunner struct {
	delay time.Duration
}

func (s *slowRunner) RunAgent(_ context.Context, prompt string) (string, error) {
	time.Sleep(s.delay)
	return "ok-" + prompt, nil
}

// countingRunner 记录同时运行的最大并发数。
type countingRunner struct {
	mu     sync.Mutex
	active int
	max    int
}

func (c *countingRunner) RunAgent(_ context.Context, prompt string) (string, error) {
	c.mu.Lock()
	c.active++
	if c.active > c.max {
		c.max = c.active
	}
	c.mu.Unlock()
	time.Sleep(20 * time.Millisecond)
	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	return "ok", nil
}

func TestRunParallel(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"a": "ra", "b": "rb", "c": "rc"}}
	r := run(t, StartRequest{
		Meta:   Meta{Name: "para", Description: "concurrent fan-out"},
		Script: `return await parallel([() => agent("a"), () => agent("b"), () => agent("c")]);`,
		Runner: fr,
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop reason = %q, err = %s", r.StopReason, r.Error)
	}
	arr, _ := r.Value.([]any)
	if len(arr) != 3 || arr[0] != "ra" || arr[1] != "rb" || arr[2] != "rc" {
		t.Fatalf("value = %+v", r.Value)
	}
	if r.AgentsStarted != 3 || len(fr.calls) != 3 {
		t.Fatalf("agents = %d, calls = %v", r.AgentsStarted, fr.calls)
	}
}

// TestRunParallelConcurrency 验证并行扇出真并发：4 个各 60ms 的 agent，
// 串行需 240ms，并发应明显更快（阈值 3 个延迟）。
func TestRunParallelConcurrency(t *testing.T) {
	fr := &slowRunner{delay: 60 * time.Millisecond}
	start := time.Now()
	r := run(t, StartRequest{
		Meta:   Meta{Name: "conc", Description: "concurrency check"},
		Script: `return await parallel([() => agent("a"), () => agent("b"), () => agent("c"), () => agent("d")]);`,
		Runner: fr,
	})
	elapsed := time.Since(start)
	if r.StopReason != StopCompleted {
		t.Fatalf("got %+v", r)
	}
	if elapsed > 3*fr.delay {
		t.Fatalf("parallel took %v, expected concurrent fan-out", elapsed)
	}
}

// TestRunParallelConcurrencyLimit 验证 MaxConcurrentAgents 上限：5 个 agent、
// 并发上限 2 时，同时运行数不得超过 2。
func TestRunParallelConcurrencyLimit(t *testing.T) {
	fr := &countingRunner{}
	r := run(t, StartRequest{
		Meta:                Meta{Name: "lim", Description: "concurrency limit"},
		Script:              `return await parallel([() => agent("a"), () => agent("b"), () => agent("c"), () => agent("d"), () => agent("e")]);`,
		Runner:              fr,
		MaxConcurrentAgents: 2,
	})
	if r.StopReason != StopCompleted || r.AgentsStarted != 5 {
		t.Fatalf("got %+v", r)
	}
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if fr.max > 2 {
		t.Fatalf("max concurrent = %d, want <= 2", fr.max)
	}
}

// TestRunParallelValidation 校验 parallel 参数契约：非数组、非函数条目、超单次条目上限。
func TestRunParallelValidation(t *testing.T) {
	fr := &fakeRunner{}
	cases := []struct{ script, code, want string }{
		{`return await parallel("no");`, ErrInvalidArgument, "requires an array"},
		{`return await parallel([3]);`, ErrInvalidArgument, "item 0 is not a function"},
		{`return await parallel([() => 1, () => 2, () => 3]);`, ErrItemCap, "maxItemsPerCall"},
	}
	for _, c := range cases {
		r := run(t, StartRequest{
			Meta:            Meta{Name: "pv", Description: "x"},
			Script:          c.script,
			Runner:          fr,
			MaxItemsPerCall: 2,
		})
		if r.StopReason != StopError || !strings.Contains(r.Error, c.code) || !strings.Contains(r.Error, c.want) {
			t.Fatalf("%q = %+v", c.script, r)
		}
	}
}

// TestRunParallelFatalEscapes 验证致命错误逸出 parallel（不会变成逐项 null）：
// parallel 内 agent 超总量上限 → AGENT_CAP 以 stopReason=error 结算。
func TestRunParallelFatalEscapes(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"a": "x"}}
	r := run(t, StartRequest{
		Meta:           Meta{Name: "pe", Description: "x"},
		Script:         `return await parallel([() => agent("a"), () => agent("a")]);`,
		Runner:         fr,
		MaxTotalAgents: 1,
	})
	if r.StopReason != StopError || !strings.Contains(r.Error, ErrAgentCap) {
		t.Fatalf("AGENT_CAP should escape parallel, got %+v", r)
	}
}

func TestRunPipeline(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"read a": "ra", "read b": "rb"}}
	r := run(t, StartRequest{
		Meta:   Meta{Name: "pipe", Description: "staged fan-out"},
		Script: `const answers = await pipeline(["a", "b"], (prev, item) => agent("read " + item)); return answers;`,
		Runner: fr,
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("stop reason = %q, err = %s", r.StopReason, r.Error)
	}
	arr, _ := r.Value.([]any)
	if len(arr) != 2 || arr[0] != "ra" || arr[1] != "rb" {
		t.Fatalf("value = %+v", r.Value)
	}
	if r.AgentsStarted != 2 || len(fr.calls) != 2 {
		t.Fatalf("agents = %d, calls = %v", r.AgentsStarted, fr.calls)
	}
}

// TestRunPipelinePreviousAndIndex 验证 stage 签名 (previous, item, index)：
// previous 为上一 stage 输出（首个 stage 为 item 本身）。
func TestRunPipelinePreviousAndIndex(t *testing.T) {
	r := run(t, StartRequest{
		Meta:   Meta{Name: "pipe2", Description: "prev chain"},
		Script: `return await pipeline([10, 20], (prev, item, i) => prev + item, (prev, item, i) => prev * (i + 1));`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("got %+v", r)
	}
	// item10: stage1=10+10=20, stage2=20*(0+1)=20；item20: stage1=20+20=40, stage2=40*(1+1)=80
	arr, _ := r.Value.([]any)
	if len(arr) != 2 || arr[0] != int64(20) || arr[1] != int64(80) {
		t.Fatalf("value = %+v", r.Value)
	}
}

// TestRunPipelineStageErrorNullsItem 验证普通 stage 错误 → 该 item 为 null，
// 其余 item 不受影响（对齐 DSH per-item null）。
func TestRunPipelineStageErrorNullsItem(t *testing.T) {
	r := run(t, StartRequest{
		Meta:   Meta{Name: "pipe3", Description: "item null"},
		Script: `return await pipeline([10, 20], (prev, item) => { if (item === 10) throw new Error("ordinary failure"); return "kept-" + item; });`,
		Runner: &fakeRunner{},
	})
	if r.StopReason != StopCompleted {
		t.Fatalf("got %+v", r)
	}
	arr, _ := r.Value.([]any)
	if len(arr) != 2 || arr[0] != nil || arr[1] != "kept-20" {
		t.Fatalf("value = %+v", r.Value)
	}
}

// TestRunPipelineValidation 校验 pipeline 参数契约：非数组、无 stage、
// 非函数 stage、超单次条目上限。
func TestRunPipelineValidation(t *testing.T) {
	fr := &fakeRunner{}
	cases := []struct{ script, code, want string }{
		{`return await pipeline("no", () => 1);`, ErrInvalidArgument, "requires an items array"},
		{`return await pipeline([1]);`, ErrInvalidArgument, "at least one stage"},
		{`return await pipeline([1], "x");`, ErrInvalidArgument, "stage 0 is not a function"},
		{`return await pipeline([1, 2, 3], (x) => x);`, ErrItemCap, "maxItemsPerCall"},
	}
	for _, c := range cases {
		r := run(t, StartRequest{
			Meta:            Meta{Name: "pv2", Description: "x"},
			Script:          c.script,
			Runner:          fr,
			MaxItemsPerCall: 2,
		})
		if r.StopReason != StopError || !strings.Contains(r.Error, c.code) || !strings.Contains(r.Error, c.want) {
			t.Fatalf("%q = %+v", c.script, r)
		}
	}
}

// TestRunPipelineFatalEscapes 验证致命错误逸出 pipeline：stage 内 agent
// 超总量上限 → AGENT_CAP 以 stopReason=error 结算（非 per-item null）。
func TestRunPipelineFatalEscapes(t *testing.T) {
	fr := &fakeRunner{resps: map[string]string{"a": "x"}}
	r := run(t, StartRequest{
		Meta:           Meta{Name: "pf", Description: "x"},
		Script:         `return await pipeline([1, 2], (prev, item) => agent("a"));`,
		Runner:         fr,
		MaxTotalAgents: 1,
	})
	if r.StopReason != StopError || !strings.Contains(r.Error, ErrAgentCap) {
		t.Fatalf("AGENT_CAP should escape pipeline, got %+v", r)
	}
}
