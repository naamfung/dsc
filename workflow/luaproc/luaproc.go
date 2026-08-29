// Package luaproc 是一个「可行性 spike」：证明把 workflow 的 goja(JS Promise)
// async 模型，改写到 go-lua 的 coroutine 模型是可行的。
//
// 核心验证点：agent()（异步跑子代理）、parallel()（并发 + 并发上限）、
// pipeline()（逐 item 并发 + stage 失败 → 该 item 置 null）。
//
// 实现方式：整段程序 + parallel 的每个 thunk 各占一个 coroutine，由一个
// Go 排程器多路 Resume/Yield 驱动。当 async 请求（agent/parallel）出现时，
// 排程器在这侧跑 goroutine 并把结果 resume 回对应 coroutine——本质是一个
// actor 排程器（对齐 go-lua fork 的「actor runtime」设计意图）。
//
// 注意：这是 spike，语义已尽力贴近 workflow，但并非完整对照实现
// （例如 pipeline 的致命错误整链传播未做）。验证通过与否决定后续是否正式 port。
package luaproc

import (
	"context"
	"fmt"
	"strings"
	"time"

	lua "github.com/wippyai/go-lua"
)

// Runner 模拟 workflow 的 agent 运行器（spike 用假实现即可）。
type Runner interface {
	RunAgent(ctx context.Context, prompt string) (string, error)
}

// Options 一次运行配置。
type Options struct {
	Script              string
	Runner              Runner
	MaxConcurrentAgents int
	Timeout             time.Duration
}

// 停止原因。
const (
	StopCompleted = "completed"
	StopError     = "error"
	StopCancelled = "cancelled"
)

// Result 运行结果（Value 为 fromLua 的 Go 值）。
type Result struct {
	Value      any
	StopReason string
	Error      string
}

// 排程器状态。
type sched struct {
	L      *lua.LState
	opts   Options
	ctx    context.Context
	sem    chan struct{}
	ready  []*unit          // 可 resume 的 coroutine 队列
	wait   map[string]*unit // agentID -> 等待中的 coroutine
	groups map[string]*group
	doneCh chan jobDone
	final  any
	nextID int
}

// unit 一个待驱动的 coroutine 及其 resume 参数。
type unit struct {
	th   *lua.LState
	fn   *lua.LFunction
	args []lua.LValue
	grp  string // 所属 parallel 组 id（顶层为空）
	idx  int    // 在组内的序号
}

// jobDone agent goroutine 完成回传。
type jobDone struct {
	id  string
	val lua.LValue
	err error
}

// group 一个待完成的 parallel 组。
type group struct {
	parent    *unit
	results   []lua.LValue
	remaining int
}

// agentReq / parallelReq：coroutine yield 抛给排程器的请求。
type agentReq struct {
	id     string
	prompt string
}
type parallelReq struct {
	id     string
	thunks []*lua.LFunction
}

// Run 执行一段 go-lua 脚本。返回停止原因与最终值。
func Run(ctx context.Context, opts Options) Result {
	if strings.TrimSpace(opts.Script) == "" {
		return Result{StopReason: StopError, Error: "empty script"}
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	capN := opts.MaxConcurrentAgents
	if capN <= 0 {
		capN = 4
	}

	s := &sched{
		opts:   opts,
		ctx:    ctx,
		sem:    make(chan struct{}, capN),
		wait:   map[string]*unit{},
		groups: map[string]*group{},
		doneCh: make(chan jobDone, 32),
	}
	s.L = lua.NewState()
	defer s.L.Close()
	lua.OpenBase(s.L)
	lua.OpenString(s.L)
	lua.OpenTable(s.L)
	lua.OpenMath(s.L)

	s.installBindings()

	// pipeline 的 Lua 装扮（构建在平行 parallel 之上）：逐 item 跑同一组 stage，
	// 任一 stage 抛错 → 该 item 置 null（对齐 workflow pipeline 语义）。
	if err := s.L.DoString(`
function __pipeline_run(items, ...)
  local stages = {...}
  local fns = {}
  for i = 1, #items do
    local it = items[i]
    fns[i] = function()
      local value = it
      for si = 1, #stages do
        local ok, nextval = pcall(stages[si], value, it, i)
        if not ok then return nil end
        value = nextval
      end
      return value
    end
  end
  return parallel(fns)
end
function pipeline(items, ...) return __pipeline_run(items, ...) end
`); err != nil {
		return Result{StopReason: StopError, Error: "pipeline preamble: " + err.Error()}
	}

	// 顶层入口函数（脚本顶层 return 即返回）。
	if err := s.L.DoString("function __entry()\n" + opts.Script + "\nend"); err != nil {
		return Result{StopReason: StopError, Error: "script parse error: " + err.Error()}
	}
	top := s.L.GetGlobal("__entry").(*lua.LFunction)
	co := s.L.NewThreadWithContext(ctx)
	s.ready = append(s.ready, &unit{th: co, fn: top})

	return s.loop()
}

// installBindings 注册 agent / parallel 两个顶层 Go 绑定。
func (s *sched) installBindings() {
	// agent(prompt)：异步跑子代理。
	agent := func(L *lua.LState) int {
		prompt := L.CheckString(1)
		if s.opts.Runner == nil {
			L.RaiseError("agent() unavailable: no runner configured")
		}
		s.nextID++
		req := &agentReq{id: fmt.Sprintf("a%d", s.nextID), prompt: prompt}
		ud := L.NewUserData()
		ud.Value = req
		return L.Yield(ud)
	}
	s.L.SetGlobal("agent", s.L.NewFunction(agent))

	// parallel(thunks)：并发跑一组函数，全体完成后返回结果数组。
	parallel := func(L *lua.LState) int {
		arr := L.CheckTable(1)
		n := arr.Len()
		thunks := make([]*lua.LFunction, n)
		for i := 1; i <= n; i++ {
			lf, ok := arr.RawGetInt(i).(*lua.LFunction)
			if !ok {
				L.RaiseError("parallel: item %d is not a function", i)
			}
			thunks[i-1] = lf
		}
		s.nextID++
		req := &parallelReq{id: fmt.Sprintf("p%d", s.nextID), thunks: thunks}
		ud := L.NewUserData()
		ud.Value = req
		return L.Yield(ud)
	}
	s.L.SetGlobal("parallel", s.L.NewFunction(parallel))
}

// loop 事件循环：驱动 ready 队列，处理 yield 请求与 goroutine 回传。
func (s *sched) loop() Result {
	for {
		if len(s.ready) == 0 && len(s.wait) == 0 {
			break
		}
		if len(s.ready) == 0 {
			select {
			case <-s.ctx.Done():
				return Result{StopReason: StopCancelled, Error: "cancelled"}
			case j := <-s.doneCh:
				s.deliverDone(j)
			}
			continue
		}
		u := s.ready[0]
		s.ready = s.ready[1:]
		st, vals, err := s.L.Resume(u.th, u.fn, u.args...)
		switch st {
		case lua.ResumeOK:
			s.finish(u, vals)
		case lua.ResumeYield:
			if rerr := s.onRequest(u, vals[0]); rerr != nil {
				return Result{StopReason: StopError, Error: rerr.Error()}
			}
		default: // ResumeError
			if s.ctx.Err() != nil {
				return Result{StopReason: StopCancelled, Error: "cancelled"}
			}
			return Result{StopReason: StopError, Error: err.Error()}
		}
	}
	return Result{Value: s.final, StopReason: StopCompleted}
}

// onRequest 处理一次 yield：agent 请求 或 parallel 组创建。
func (s *sched) onRequest(u *unit, v lua.LValue) error {
	ud, ok := v.(*lua.LUserData)
	if !ok {
		return fmt.Errorf("unexpected yield value")
	}
	switch req := ud.Value.(type) {
	case *agentReq:
		s.wait[req.id] = u // 先把 coroutine 停泊
		go s.runAgent(req)
	case *parallelReq:
		g := &group{parent: u, results: make([]lua.LValue, len(req.thunks)), remaining: len(req.thunks)}
		s.groups[req.id] = g
		for i, fn := range req.thunks {
			th := s.L.NewThreadWithContext(s.ctx)
			s.ready = append(s.ready, &unit{th: th, fn: fn, grp: req.id, idx: i})
		}
	default:
		return fmt.Errorf("unexpected request type")
	}
	return nil
}

// runAgent 在 goroutine 里跑子代理（受限并发 semaphore）。
func (s *sched) runAgent(req *agentReq) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
		res, err := s.opts.Runner.RunAgent(s.ctx, req.prompt)
		var val lua.LValue = lua.LString(res)
		if err != nil {
			val = lua.LNil
		}
		s.doneCh <- jobDone{id: req.id, val: val, err: err}
	case <-s.ctx.Done():
		s.doneCh <- jobDone{id: req.id, val: lua.LNil, err: s.ctx.Err()}
	}
}

// deliverDone 把 agent 结果 resume 回停泊的 coroutine。
func (s *sched) deliverDone(j jobDone) {
	u, ok := s.wait[j.id]
	if !ok {
		return
	}
	delete(s.wait, j.id)
	u.args = []lua.LValue{j.val}
	s.ready = append(s.ready, u)
}

// finish 一个 coroutine 正常结束：顶层记 final；组内子项回填结果并在
// 全体完成后 resume 父 coroutine。
func (s *sched) finish(u *unit, vals []lua.LValue) {
	val := lua.LNil
	if len(vals) > 0 {
		val = vals[0]
	}
	if u.grp == "" {
		// 顶层完成。
		s.final = fromLua(val)
		return
	}
	g := s.groups[u.grp]
	g.results[u.idx] = val
	g.remaining--
	if g.remaining == 0 {
		tbl := s.L.NewTable()
		for i, r := range g.results {
			// 信封 {ok, value}：nil 结果也会占位，保证数组位置（Lua 表存不了裸 nil）。
			tbl.RawSetInt(i+1, envTable(s.L, r))
		}
		p := g.parent
		delete(s.groups, u.grp)
		s.ready = append(s.ready, &unit{th: p.th, fn: p.fn, args: []lua.LValue{tbl}})
	}
}

// envTable 把结果包成信封 {ok=<bool>, value=<值>}；nil → {ok=false}（不设 value）。
func envTable(L *lua.LState, r lua.LValue) *lua.LTable {
	t := L.NewTable()
	if r == lua.LNil {
		t.RawSetString("ok", lua.LFalse)
	} else {
		t.RawSetString("ok", lua.LTrue)
		t.RawSetString("value", r)
	}
	return t
}

// fromLua 把 LValue 转成 JSON 兼容的 Go 值（spike 简化版）。
func fromLua(v lua.LValue) any {
	switch t := v.(type) {
	case *lua.LTable:
		return tableToGo(t)
	case lua.LString:
		return string(t)
	case lua.LNumber:
		return float64(t)
	case lua.LInteger:
		return int64(t)
	case lua.LBool:
		return bool(t)
	default:
		if v == lua.LNil {
			return nil
		}
		return v.String()
	}
}

// tableToGo 简化转换：全部键为正整数 → 按最大下标生成 []any（缺位补 nil，
// 因为 Lua 表存不了 nil，中段 nil 会开窿）；否则 → map[string]any。
func tableToGo(t *lua.LTable) any {
	isArray := true
	arrLen := 0
	t.ForEach(func(k, _ lua.LValue) {
		n, ok := intKey(k)
		if !ok || n < 1 {
			isArray = false
			return
		}
		if n > arrLen {
			arrLen = n
		}
	})
	if isArray && arrLen > 0 {
		out := make([]any, arrLen)
		for i := 1; i <= arrLen; i++ {
			if rv := t.RawGetInt(i); rv != lua.LNil {
				out[i-1] = fromLua(rv)
			}
		}
		return out
	}
	out := make(map[string]any, 4)
	t.ForEach(func(k, v lua.LValue) { out[k.String()] = fromLua(v) })
	return out
}

// intKey 把整数下标 key（LInteger 或整数值 LNumber）归一成 int。
func intKey(k lua.LValue) (int, bool) {
	switch t := k.(type) {
	case lua.LInteger:
		return int(int64(t)), true
	case lua.LNumber:
		f := float64(t)
		if f != float64(int(f)) {
			return 0, false
		}
		return int(f), true
	}
	return 0, false
}
