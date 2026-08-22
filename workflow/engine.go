package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
)

// newID 生成运行 id：时间戳毫秒。
func newID() string {
	return fmt.Sprintf("wf-%d", time.Now().UnixMilli())
}

// emptyProgram 用于在事件循环中触发 goja job queue（leave() drain），
// 使 promise resolve 后的 await 续行得以执行。
var emptyProgram = goja.MustCompile("drain.js", "", false)

// agentJob agent goroutine → VM goroutine 的结果回传。
// resolve 由 goja.NewPromise 返回，必须仅在 VM goroutine 上调用（非 goroutine-safe）。
type agentJob struct {
	resolve func(interface{}) error
	result  string
	err     error
}

// checkScriptSyntax 语法预检（Start 同步执行）：只编译包装后的脚本，不运行。
func checkScriptSyntax(script string) error {
	wrapped := "(async function(){\n" + script + "\n})()"
	_, err := goja.Compile("workflow.js", wrapped, false)
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

// settle 执行脚本并结算。脚本以 async IIFE 包装：agent()/parallel() 返回
// Promise，await 时脚本挂起。取消/超时经 vm.Interrupt 中断同步段；脚本挂起
// 期间由事件循环驱动——agent goroutine 完成经 channel 回传，VM goroutine 内
// resolve 并用空 program 触发 job queue（leave() drain）推进 await 链。
func settle(ctx context.Context, req StartRequest, id string) Result {
	vm := goja.New()

	var (
		mu          sync.Mutex
		agentsCount int32
		agentSeq    int
	)
	emit := func(f func(EventSink)) {
		if req.Events != nil {
			f(req.Events)
		}
	}
	panicErr := func(code string, err error) {
		panic(vm.ToValue(&RunError{Code: code, Err: err}))
	}

	// agent 并发上限：<=0 按可用 CPU 并行度解析；信号量在 agent goroutine
	// 中获取，不阻塞 VM 线程（并发打满时若在 VM 线程获取会死锁）。
	concurrency := req.MaxConcurrentAgents
	if concurrency <= 0 {
		concurrency = runtime.GOMAXPROCS(0)
	}
	sem := make(chan struct{}, concurrency)
	jobs := make(chan agentJob, concurrency)

	// agent(prompt, options?)：异步执行一个子 agent，返回 Promise；
	// 子 agent 失败 resolve null（对齐 DSH：非基础设施失败脚本侧可见 null）。
	vm.Set("agent", func(call goja.FunctionCall) goja.Value {
		prompt := call.Argument(0).String()
		if strings.TrimSpace(prompt) == "" {
			panicErr(ErrInvalidArgument, fmt.Errorf("agent(prompt) requires a non-empty prompt"))
		}
		label := prompt
		if opt := call.Argument(1); opt != nil && opt != goja.Undefined() && !goja.IsNull(opt) {
			if o := opt.ToObject(vm); o != nil {
				if l := o.Get("label"); l != nil && l != goja.Undefined() && !goja.IsNull(l) {
					label = l.String()
				}
			}
		}
		if req.MaxTotalAgents > 0 && int(atomic.LoadInt32(&agentsCount)) >= req.MaxTotalAgents {
			panicErr(ErrAgentCap, fmt.Errorf("total agent cap %d exceeded", req.MaxTotalAgents))
		}
		mu.Lock()
		agentSeq++
		seq := agentSeq
		mu.Unlock()
		atomic.StoreInt32(&agentsCount, int32(seq))
		emit(func(s EventSink) { s.OnAgentStart(id, seq, label) })

		p, resolve, _ := vm.NewPromise()
		go func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
				result, err := req.Runner.RunAgent(ctx, prompt)
				outcome := "completed"
				if err != nil {
					outcome = "failed"
				}
				emit(func(s EventSink) { s.OnAgentEnd(id, seq, outcome) })
				jobs <- agentJob{resolve: resolve, result: result, err: err}
			case <-ctx.Done():
				// 取消/超时：子 agent 未启动视为失败（事件循环按 ctx 退出）
				emit(func(s EventSink) { s.OnAgentEnd(id, seq, "failed") })
				jobs <- agentJob{resolve: resolve, err: ctx.Err()}
			}
		}()
		return vm.ToValue(p) // *Promise 经 valueContainer 转回其底层 Object
	})

	// parallel(thunks)：在并发上限内扇出 thunk（对齐 DSH）。thunk 通常为
	// () => agent(...)，同步执行收集其 promise 后经 Promise.all 聚合，子
	// agent 底层并发运行。致命错误（INVALID_ARGUMENT / ITEM_CAP / AGENT_CAP）
	// 逸出调用，不会变成逐项 null。
	vm.Set("parallel", func(call goja.FunctionCall) goja.Value {
		v := call.Argument(0)
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) || v.ToObject(vm).ClassName() != "Array" {
			panicErr(ErrInvalidArgument, fmt.Errorf("parallel() requires an array"))
		}
		arr := v.ToObject(vm)
		length := int(arr.Get("length").ToInteger())
		maxItems := req.MaxItemsPerCall
		if maxItems <= 0 {
			maxItems = defaultMaxItemsPerCall
		}
		if length > maxItems {
			panicErr(ErrItemCap, fmt.Errorf("parallel() exceeds maxItemsPerCall (%d)", maxItems))
		}
		promises := vm.NewArray()
		for i := 0; i < length; i++ {
			thunk := arr.Get(strconv.Itoa(i))
			fn, ok := goja.AssertFunction(thunk)
			if !ok {
				panicErr(ErrInvalidArgument, fmt.Errorf("parallel() item %d is not a function", i))
			}
			ret, err := fn(goja.Undefined())
			if err != nil {
				if re := unwrapRunError(err); re != nil {
					panic(vm.ToValue(re)) // 致命错误原样逸出（如 AGENT_CAP）
				}
				panicErr("SCRIPT_RUNTIME", err)
			}
			promises.Set(strconv.Itoa(i), ret)
		}
		all, ok := goja.AssertFunction(vm.Get("Promise").ToObject(vm).Get("all"))
		if !ok {
			panicErr("SCRIPT_RUNTIME", fmt.Errorf("Promise.all unavailable"))
		}
		// this 必须指向 Promise 构造器（Promise.all 内部经 speciesConstructor 取 this）
		res, err := all(vm.Get("Promise"), promises)
		if err != nil {
			panicErr("SCRIPT_RUNTIME", err)
		}
		return res
	})

	// dsc_is_fatal：供 pipeline 脚本侧区分致命错误（Go 钩子 panic 的 *RunError）
	// 与普通错误。*RunError 经 Export 还原（对齐 DSH 的 instanceof 隔离：脚本
	// 伪造的 {Code:...} 对象不能冒充致命错误，落入 per-item null）。
	vm.Set("dsc_is_fatal", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(valueToRunError(call.Argument(0)) != nil)
	})

	// dsc_pipeline_run：pipeline 的 JS 运行时（一次定义、多次调用）。每个 item
	// 独立串行跑完全部 stages（无跨阶段屏障），item 之间并发（Promise.all）；
	// stage 签名 (previous, item, index)，previous 为上一 stage 输出（首个 stage
	// 为 item 本身）。stage 抛普通错误 → 该 item 为 null 并跳过其剩余 stages；
	// 致命错误（dsc_is_fatal）传播整链。
	if _, err := vm.RunString(`
function dsc_pipeline_run(items, stages) {
  return Promise.all(items.map(async (item, index) => {
    let value = item;
    try {
      for (const stage of stages) {
        value = await stage(value, item, index);
      }
      return value;
    } catch (e) {
      if (dsc_is_fatal(e)) throw e;
      return null;
    }
  }));
}
`); err != nil {
		return classifyError(err, agentsCount) // 理论不发生（固定源码）
	}

	// pipeline(items, ...stages)：逐项 stage 链（对齐 DSH），无跨阶段屏障；
	// 致命错误（INVALID_ARGUMENT / ITEM_CAP / AGENT_CAP）逸出调用。
	vm.Set("pipeline", func(call goja.FunctionCall) goja.Value {
		itemsV := call.Argument(0)
		if itemsV == nil || goja.IsUndefined(itemsV) || goja.IsNull(itemsV) || itemsV.ToObject(vm).ClassName() != "Array" {
			panicErr(ErrInvalidArgument, fmt.Errorf("pipeline() requires an items array"))
		}
		itemsObj := itemsV.ToObject(vm)
		length := int(itemsObj.Get("length").ToInteger())
		maxItems := req.MaxItemsPerCall
		if maxItems <= 0 {
			maxItems = defaultMaxItemsPerCall
		}
		if length > maxItems {
			panicErr(ErrItemCap, fmt.Errorf("pipeline() exceeds maxItemsPerCall (%d)", maxItems))
		}
		stages := call.Arguments[1:]
		if len(stages) == 0 {
			panicErr(ErrInvalidArgument, fmt.Errorf("pipeline() requires at least one stage function"))
		}
		for i, s := range stages {
			if _, ok := goja.AssertFunction(s); !ok {
				panicErr(ErrInvalidArgument, fmt.Errorf("pipeline() stage %d is not a function", i))
			}
		}
		runFn, ok := goja.AssertFunction(vm.Get("dsc_pipeline_run"))
		if !ok {
			panicErr("SCRIPT_RUNTIME", fmt.Errorf("dsc_pipeline_run unavailable"))
		}
		stageVals := make([]any, len(stages))
		for i, s := range stages {
			stageVals[i] = s
		}
		res, err := runFn(goja.Undefined(), itemsV, vm.NewArray(stageVals...))
		if err != nil {
			if re := unwrapRunError(err); re != nil {
				panic(vm.ToValue(re)) // 致命错误原样逸出（如 AGENT_CAP）
			}
			panicErr("SCRIPT_RUNTIME", err)
		}
		return res
	})

	// phase(title)：声明了 meta.phases 时须精确匹配，否则报错。
	vm.Set("phase", func(call goja.FunctionCall) goja.Value {
		title := call.Argument(0).String()
		if strings.TrimSpace(title) == "" {
			panicErr(ErrInvalidArgument, fmt.Errorf("phase(title) requires a non-empty title"))
		}
		if len(req.Meta.Phases) > 0 {
			found := false
			for _, p := range req.Meta.Phases {
				if p.Title == title {
					found = true
					break
				}
			}
			if !found {
				panicErr(ErrInvalidArgument, fmt.Errorf("phase %q not declared in meta.phases", title))
			}
		}
		emit(func(s EventSink) { s.OnPhase(id, title) })
		return goja.Undefined()
	})

	// log(msg)：进度叙述。
	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		emit(func(s EventSink) { s.OnLog(id, call.Argument(0).String()) })
		return goja.Undefined()
	})

	// args 全局：请求参数原样暴露（无参数时为空对象）。
	if req.Args != nil {
		vm.Set("args", vm.ToValue(req.Args))
	} else {
		vm.Set("args", vm.ToValue(map[string]any{}))
	}

	// 取消/超时：中断脚本执行（同步段生效；脚本挂起时事件循环按 ctx 退出）。
	interrupt := &RunError{Code: ErrCancelled, Err: fmt.Errorf("workflow run cancelled")}
	stopInterrupt := context.AfterFunc(ctx, func() { vm.Interrupt(interrupt) })
	defer stopInterrupt()

	wrapped := "(async function(){\n" + req.Script + "\n})()"
	val, err := vm.RunString(wrapped)
	if err != nil {
		return classifyError(err, agentsCount)
	}
	promise, ok := val.Export().(*goja.Promise)
	if !ok {
		// 理论不发生（async IIFE 恒返回 Promise），兜底直接物化
		value, merr := materialize(val)
		if merr != nil {
			return Result{StopReason: StopError, Error: merr.Error(), AgentsStarted: int(agentsCount)}
		}
		return Result{Value: value, StopReason: StopCompleted, AgentsStarted: int(agentsCount)}
	}
	// 事件循环：驱动 agent 结果回传与 goja job queue（await 续行）。
	for promise.State() == goja.PromiseStatePending {
		select {
		case <-ctx.Done():
			return Result{StopReason: StopCancelled, Error: interrupt.Error(), AgentsStarted: int(agentsCount)}
		case j := <-jobs:
			if j.err != nil {
				_ = j.resolve(nil) // 子 agent 失败 → null（对齐 DSH）
			} else {
				_ = j.resolve(j.result)
			}
			// resolve 后 promise 链 job 入队，用空 program 触发 leave() drain；
			// drain 过程中可能发起新的 agent()，产生新的 job 继续循环。
			if _, derr := vm.RunProgram(emptyProgram); derr != nil {
				return classifyError(derr, agentsCount)
			}
		}
	}
	switch promise.State() {
	case goja.PromiseStateFulfilled:
		value, merr := materialize(promise.Result())
		if merr != nil {
			return Result{StopReason: StopError, Error: merr.Error(), AgentsStarted: int(agentsCount)}
		}
		return Result{Value: value, StopReason: StopCompleted, AgentsStarted: int(agentsCount)}
	default: // Rejected（Rejected 时 Result() 即 rejection 值）
		if re := valueToRunError(promise.Result()); re != nil {
			return classifyError(re, agentsCount)
		}
		msg := ""
		if r := promise.Result(); r != nil {
			msg = r.ToString().String()
		}
		return classifyError(fmt.Errorf("%s", msg), agentsCount)
	}
}

// classifyError 把脚本抛错/中断归类为运行结果：
// 钩子 panic 的 *RunError（或经 Exception 包装）原样保留其 code；
// 取消/超时中断按 CANCELLED 归类，其余为 SCRIPT_RUNTIME。
func classifyError(err error, agents int32) Result {
	re := unwrapRunError(err)
	if re == nil {
		if e, ok := err.(*RunError); ok {
			re = e
		}
	}
	if re == nil {
		// goja 中断（vm.Interrupt）把错误包成带位置后缀的异常，按 code 前缀识别
		if strings.Contains(err.Error(), ErrCancelled) {
			re = &RunError{Code: ErrCancelled, Err: err}
		} else {
			re = &RunError{Code: "SCRIPT_RUNTIME", Err: err}
		}
	}
	reason := StopError
	if re.Code == ErrCancelled {
		reason = StopCancelled
	}
	return Result{StopReason: reason, Error: re.Error(), AgentsStarted: int(agents)}
}

// unwrapRunError 尝试从 goja 异常中还原钩子 panic 的 *RunError。
func unwrapRunError(err error) *RunError {
	if ex, ok := err.(*goja.Exception); ok {
		if v := ex.Value(); v != nil {
			if re, ok := v.Export().(*RunError); ok {
				return re
			}
		}
	}
	return nil
}

// valueToRunError 从 goja 值还原 *RunError（promise rejection 值）。
func valueToRunError(v goja.Value) *RunError {
	if v == nil || goja.IsNull(v) || goja.IsUndefined(v) {
		return nil
	}
	if re, ok := v.Export().(*RunError); ok {
		return re
	}
	return nil
}

// materialize 物化脚本返回值：必须是普通 JSON 数据，否则 RESULT_UNSERIALIZABLE。
func materialize(v goja.Value) (any, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, nil
	}
	exported := v.Export()
	if _, err := json.Marshal(exported); err != nil {
		return nil, &RunError{Code: ErrResultUnserializable, Err: fmt.Errorf("script returned non-JSON value: %v", err)}
	}
	return exported, nil
}
