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

func (r *recSink) OnStart(string, Meta)        { r.mu.Lock(); r.starts++; r.mu.Unlock() }
func (r *recSink) OnPhase(_ string, t string)  { r.mu.Lock(); r.phases = append(r.phases, t); r.mu.Unlock() }
func (r *recSink) OnLog(_ string, m string)    { r.mu.Lock(); r.logs = append(r.logs, m); r.mu.Unlock() }
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
		Script: `const x = agent("a"); const y = agent("b"); return [x, y];`,
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
		Script: `const x = agent("boom"); return x === null;`,
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
		Script:         `agent("a"); agent("a"); return 1;`,
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
		Meta:   Meta{Name: "hang", Description: "x"},
		Script: `while (Date.now() < 1e15) {} return 1;`,
		Runner: fr,
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
