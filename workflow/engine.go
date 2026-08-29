package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"

	lua "github.com/wippyai/go-lua"
	"github.com/wippyai/go-lua/compiler/parse"
)

// newID 生成运行 id：时间戳毫秒。
func newID() string {
	return fmt.Sprintf("wf-%d", time.Now().UnixMilli())
}

// checkScriptSyntax 语法预检（Start 同步执行）：只编译包装后的脚本，不运行。
func checkScriptSyntax(script string) error {
	_, err := parse.Parse(strings.NewReader("function __entry()\n"+script+"\nend"), "workflow.lua")
	return err
}

// execute 运行协程：应用超时后结算并投递结果。
func execute(ctx context.Context, req StartRequest, id string, timeout time.Duration, ch chan<- Result) {
	defer close(ch)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if req.Events != nil {
		req.Events.OnStart(id, req.Meta)
	}
	r := settle(ctx, req, id)
	if req.Events != nil {
		req.Events.OnEnd(id, r)
	}
	ch <- r
}

// 排程器状态：actor 排程器（对齐 luaproc spike），接入 workflow 契约
// （EventSink / MaxTotalAgents / MaxItemsPerCall / fatal 错误码）。
type sched struct {
	ctx        context.Context
	req        StartRequest
	id         string
	L          *lua.LState
	sem        chan struct{}
	ready      []*unit
	wait       map[string]*unit
	groups     map[string]*group
	doneCh     chan jobDone
	agents     int
	finalValue any
	nextID     int
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
}

// group 一个待完成的 parallel 组。
type group struct {
	parent    *unit
	results   []lua.LValue
	remaining int
}

// agentReq / parallelReq / fatalReq：coroutine yield 抛给排程器的请求。
type agentReq struct {
	id     string
	seq    int
	prompt string
}
type parallelReq struct {
	id     string
	thunks []*lua.LFunction
}
type fatalReq struct {
	err error
}

// settle 执行脚本并结算。整段脚本 + parallel 的每个 thunk 各占一个 coroutine，
// 由 Go 排程器驱动：agent() 请求经 goroutine（受并发上限）执行后 resume 回原
// coroutine；parallel 组等全体子项完成才 resume 父 coroutine。取消/超时经 ctx。
func settle(ctx context.Context, req StartRequest, id string) Result {
	capN := req.MaxConcurrentAgents
	if capN <= 0 {
		capN = runtime.GOMAXPROCS(0)
	}
	s := &sched{
		ctx:    ctx,
		req:    req,
		id:     id,
		sem:    make(chan struct{}, capN),
		wait:   map[string]*unit{},
		groups: map[string]*group{},
		doneCh: make(chan jobDone, 128),
	}
	s.L = lua.NewState()
	defer s.L.Close()
	lua.OpenBase(s.L)
	lua.OpenString(s.L)
	lua.OpenTable(s.L)
	lua.OpenMath(s.L)

	s.installBindings()

	// __pipeline_makefn：为 pipeline 的单个 item 构造 thunk（closure 捕获
	// it / idx / stages）；stage 签名 (previous, item, index)，index 0-based。
	if err := s.L.DoString(`
function __pipeline_makefn(it, idx, stages)
  return function()
    local value = it
    for si = 1, #stages do
      local ok, nextval = pcall(stages[si], value, it, idx)
      if not ok then return nil end
      value = nextval
    end
    return value
  end
end
`); err != nil {
		return Result{StopReason: StopError, Error: "pipeline preamble: " + err.Error()}
	}

	if err := s.L.DoString("function __entry()\n" + req.Script + "\nend"); err != nil {
		return Result{StopReason: StopError, Error: "script parse error: " + err.Error()}
	}
	top := s.L.GetGlobal("__entry").(*lua.LFunction)
	co := s.L.NewThreadWithContext(ctx)
	s.ready = append(s.ready, &unit{th: co, fn: top})

	return s.loop()
}

// installBindings 注册 agent / parallel / phase / log / args。
func (s *sched) installBindings() {
	// agent(prompt, options?)：异步跑子代理；结果 string，失败 → nil。
	agent := func(L *lua.LState) int {
		if s.req.Runner == nil {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("runner is required")})
		}
		prompt := strings.TrimSpace(L.CheckString(1))
		if prompt == "" {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("agent(prompt) requires a non-empty prompt")})
		}
		label := prompt
		if o := L.Get(2); o != lua.LNil {
			if ot, ok := o.(*lua.LTable); ok {
				if l := ot.RawGetString("label"); l != lua.LNil {
					label = l.String()
				}
			}
		}
		if s.req.MaxTotalAgents > 0 && s.agents >= s.req.MaxTotalAgents {
			return s.yieldFatal(L, &RunError{Code: ErrAgentCap, Err: fmt.Errorf("total agent cap %d exceeded", s.req.MaxTotalAgents)})
		}
		s.agents++
		seq := s.agents
		s.emit(func(sf EventSink) { sf.OnAgentStart(s.id, seq, label) })

		s.nextID++
		req := &agentReq{id: fmt.Sprintf("a%d", s.nextID), seq: seq, prompt: prompt}
		ud := L.NewUserData()
		ud.Value = req
		return L.Yield(ud)
	}
	s.L.SetGlobal("agent", s.L.NewFunction(agent))

	// parallel(thunks)：并发跑一组函数，全体完成后返回 {ok,value} 信封数组。
	parallel := func(L *lua.LState) int {
		arr, ok := L.Get(1).(*lua.LTable)
		if !ok {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("parallel() requires an array")})
		}
		n := arr.Len()
		maxItems := s.req.MaxItemsPerCall
		if maxItems <= 0 {
			maxItems = defaultMaxItemsPerCall
		}
		if n > maxItems {
			return s.yieldFatal(L, &RunError{Code: ErrItemCap, Err: fmt.Errorf("parallel() exceeds maxItemsPerCall (%d)", maxItems)})
		}
		thunks := make([]*lua.LFunction, n)
		for i := 1; i <= n; i++ {
			lf, ok := arr.RawGetInt(i).(*lua.LFunction)
			if !ok {
				return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("parallel() item %d is not a function", i)})
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

	// pipeline(items, ...stages)：逐 item 无跨阶段屏障地跑 stage 链；stage 签名
	// (previous, item, index)，index 0-based；普通 stage 错误 → 该 item null。
	// 参数校验在 Go 侧（产出 INVALID_ARGUMENT / ITEM_CAP fatal code），
	// 执行委托给 parallel 的组机制（并发 + 信封结果）。
	pipeline := func(L *lua.LState) int {
		items, ok := L.Get(1).(*lua.LTable)
		if !ok {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("pipeline() requires an items array")})
		}
		n := items.Len()
		maxItems := s.req.MaxItemsPerCall
		if maxItems <= 0 {
			maxItems = defaultMaxItemsPerCall
		}
		if n > maxItems {
			return s.yieldFatal(L, &RunError{Code: ErrItemCap, Err: fmt.Errorf("pipeline() exceeds maxItemsPerCall (%d)", maxItems)})
		}
		if L.GetTop() < 2 {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("pipeline() requires at least one stage")})
		}
		stages := L.NewTable()
		for i := 2; i <= L.GetTop(); i++ {
			if _, ok := L.Get(i).(*lua.LFunction); !ok {
				return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("pipeline() stage %d is not a function", i-1)})
			}
			stages.RawSetInt(i-1, L.Get(i))
		}
		makeFn, _ := L.GetGlobal("__pipeline_makefn").(*lua.LFunction)
		thunks := make([]*lua.LFunction, 0, n)
		for i := 1; i <= n; i++ {
			it := items.RawGetInt(i)
			idx := i - 1 // 0-based index（对齐 DSH stage 签名）
			L.Push(makeFn)
			L.Push(it)
			L.Push(lua.LInteger(idx))
			L.Push(stages)
			if cerr := L.PCall(3, 1, nil); cerr != nil {
				return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("pipeline() thunk build: %v", cerr)})
			}
			thunk, ok := L.Get(-1).(*lua.LFunction)
			L.Pop(1)
			if !ok {
				return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("pipeline() thunk not a function")})
			}
			thunks = append(thunks, thunk)
		}
		s.nextID++
		req := &parallelReq{id: fmt.Sprintf("p%d", s.nextID), thunks: thunks}
		ud := L.NewUserData()
		ud.Value = req
		return L.Yield(ud)
	}
	s.L.SetGlobal("pipeline", s.L.NewFunction(pipeline))

	// phase(title)：声明了 meta.phases 时须精确匹配。
	phase := func(L *lua.LState) int {
		title := L.CheckString(1)
		if strings.TrimSpace(title) == "" {
			return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("phase(title) requires a non-empty title")})
		}
		if len(s.req.Meta.Phases) > 0 {
			found := false
			for _, p := range s.req.Meta.Phases {
				if p.Title == title {
					found = true
					break
				}
			}
			if !found {
				return s.yieldFatal(L, &RunError{Code: ErrInvalidArgument, Err: fmt.Errorf("phase %q not declared in meta.phases", title)})
			}
		}
		s.emit(func(sf EventSink) { sf.OnPhase(s.id, title) })
		return 0
	}
	s.L.SetGlobal("phase", s.L.NewFunction(phase))

	// log(msg)：进度叙述。
	log := func(L *lua.LState) int {
		s.emit(func(sf EventSink) { sf.OnLog(s.id, L.CheckString(1)) })
		return 0
	}
	s.L.SetGlobal("log", s.L.NewFunction(log))

	// args 全局。
	if s.req.Args != nil {
		s.L.SetGlobal("args", toLua(s.L, s.req.Args))
	} else {
		s.L.SetGlobal("args", s.L.NewTable())
	}
}

// yieldFatal 把致命错误作为 yield 请求抛给排程器（保持控制权在 Go 侧）。
func (s *sched) yieldFatal(L *lua.LState, err error) int {
	ud := L.NewUserData()
	ud.Value = &fatalReq{err: err}
	return L.Yield(ud)
}

// emit 事件分发（nil 安全）。
func (s *sched) emit(f func(EventSink)) {
	if s.req.Events != nil {
		f(s.req.Events)
	}
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
				return Result{StopReason: StopCancelled, Error: "workflow run cancelled", AgentsStarted: s.agents}
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
				return Result{StopReason: StopError, Error: rerr.Error(), AgentsStarted: s.agents}
			}
		default: // ResumeError
			if s.ctx.Err() != nil {
				return Result{StopReason: StopCancelled, Error: "workflow run cancelled", AgentsStarted: s.agents}
			}
			return Result{StopReason: StopError, Error: err.Error(), AgentsStarted: s.agents}
		}
	}
	return s.finishFinal()
}

// onRequest 处理一次 yield：agent / parallel / fatal / 未知。
func (s *sched) onRequest(u *unit, v lua.LValue) error {
	ud, ok := v.(*lua.LUserData)
	if !ok {
		return fmt.Errorf("unexpected yield value")
	}
	switch req := ud.Value.(type) {
	case *agentReq:
		s.wait[req.id] = u
		go s.runAgent(req)
	case *parallelReq:
		g := &group{parent: u, results: make([]lua.LValue, len(req.thunks)), remaining: len(req.thunks)}
		s.groups[req.id] = g
		for i, fn := range req.thunks {
			th := s.L.NewThreadWithContext(s.ctx)
			s.ready = append(s.ready, &unit{th: th, fn: fn, grp: req.id, idx: i})
		}
	case *fatalReq:
		return req.err
	default:
		return fmt.Errorf("unexpected request type")
	}
	return nil
}

// runAgent 在 goroutine 里跑子代理（受限并发 semaphore）。
func (s *sched) runAgent(req *agentReq) {
	var val lua.LValue = lua.LNil
	outcome := "failed"
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
		res, err := s.req.Runner.RunAgent(s.ctx, req.prompt)
		if err == nil {
			val = lua.LString(res)
			outcome = "completed"
		}
	case <-s.ctx.Done():
	}
	s.emit(func(sf EventSink) { sf.OnAgentEnd(s.id, req.seq, outcome) })
	s.selectSend(jobDone{id: req.id, val: val})
}

// selectSend 投递 job（ctx 取消后不阻塞）。
func (s *sched) selectSend(j jobDone) {
	select {
	case s.doneCh <- j:
	case <-s.ctx.Done():
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

// finish 一个 coroutine 正常结束：顶层记 final；组内子项回填信封结果并在
// 全体完成后 resume 父 coroutine。
func (s *sched) finish(u *unit, vals []lua.LValue) {
	val := lua.LNil
	if len(vals) > 0 {
		val = vals[0]
	}
	if u.grp == "" {
		s.finalValue = fromLua(val)
		return
	}
	g := s.groups[u.grp]
	g.results[u.idx] = envTable(s.L, val)
	g.remaining--
	if g.remaining == 0 {
		tbl := s.L.NewTable()
		for i, r := range g.results {
			tbl.RawSetInt(i+1, r)
		}
		p := g.parent
		delete(s.groups, u.grp)
		s.ready = append(s.ready, &unit{th: p.th, fn: p.fn, args: []lua.LValue{tbl}})
	}
}

// finishFinal 顶层完成后结算结果，并校验 JSON 可序列化。
func (s *sched) finishFinal() Result {
	r := Result{StopReason: StopCompleted, AgentsStarted: s.agents}
	if s.finalValue != nil {
		if _, jerr := json.Marshal(s.finalValue); jerr != nil {
			return Result{StopReason: StopError, Error: ErrResultUnserializable + ": " + jerr.Error(), AgentsStarted: s.agents}
		}
		r.Value = s.finalValue
	}
	return r
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

// fromLua 把 LValue 转成 JSON 兼容的 Go 值。
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

// tableToGo 把 Lua 表转成 Go 值：全部键为正整数 → 按最大下标生成 []any；
// 否则 → map[string]any。
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

// toLua 把 Go 值转成 LValue（供 args 绑定）。
func toLua(L *lua.LState, v any) lua.LValue {
	switch t := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		if t {
			return lua.LTrue
		}
		return lua.LFalse
	case string:
		return lua.LString(t)
	case int:
		return lua.LInteger(t)
	case int64:
		return lua.LInteger(t)
	case float64:
		if i, ok := integral(t); ok {
			return lua.LInteger(i)
		}
		return lua.LNumber(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return lua.LInteger(i)
		}
		if f, err := t.Float64(); err == nil {
			return lua.LNumber(f)
		}
		return lua.LString(t.String())
	case []any:
		tbl := L.NewTable()
		for i, e := range t {
			tbl.RawSetInt(i+1, toLua(L, e))
		}
		return tbl
	case map[string]any:
		tbl := L.NewTable()
		for k, e := range t {
			tbl.RawSetString(k, toLua(L, e))
		}
		return tbl
	default:
		if b, err := json.Marshal(t); err == nil {
			var j any
			if json.Unmarshal(b, &j) == nil {
				return toLua(L, j)
			}
			return lua.LString(string(b))
		}
		return lua.LString(fmt.Sprintf("%v", t))
	}
}

// integral 若浮点可精确表示整数则返回其整数值。
func integral(f float64) (int64, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	if math.Trunc(f) != f {
		return 0, false
	}
	return int64(f), true
}
