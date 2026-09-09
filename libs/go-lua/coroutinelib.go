package lua

import "sync"

// SpawnRequest is yielded by coroutine.spawn to signal the scheduler
type SpawnRequest struct {
	Fn *LFunction
}

func (s *SpawnRequest) String() string   { return "spawn-request" }
func (s *SpawnRequest) Type() LValueType { return LTUserData }

var spawnRequestPool = sync.Pool{
	New: func() any {
		return &SpawnRequest{}
	},
}

// ReleaseSpawnRequest returns a SpawnRequest to the pool.
func ReleaseSpawnRequest(sr *SpawnRequest) {
	sr.Fn = nil
	spawnRequestPool.Put(sr)
}

func OpenCoroutine(L *LState) int {
	mod := L.RegisterGoModule(CoroutineLibName, coFuncs).(*LTable)
	L.Push(mod)
	return 1
}

var coFuncs = map[string]LGoFunc{
	"create":  coCreate,
	"yield":   coYield,
	"resume":  coResume,
	"running": coRunning,
	"status":  coStatus,
	"wrap":    coWrap,
	"spawn":   coSpawn,
}

func coSpawn(L *LState) int {
	fn := L.CheckFunction(1)
	req := spawnRequestPool.Get().(*SpawnRequest)
	req.Fn = fn
	L.Push(req)
	return -1 // yield with the SpawnRequest
}

func coCreate(L *LState) int {
	fn := L.CheckFunction(1)
	newthread := L.NewThreadWithContext(L.ctx)
	newthread.stack.Push(callFrame{
		Fn:         fn,
		Pc:         0,
		Base:       0,
		LocalBase:  1,
		ReturnBase: 0,
		NArgs:      0,
		NRet:       MultRet,
		TailCall:   0,
	})
	L.Push(newthread)
	return 1
}

func coYield(L *LState) int {
	return -2 // -2 signals user yield (vs -1 for system yield)
}

func coResume(L *LState) int {
	th := L.CheckThread(1)
	if L.G.CurrentThread == th {
		msg := "can not resume a running thread"
		if th.wrapped {
			L.RaiseError(msg)
			return 0
		}
		L.Push(LFalse)
		L.Push(LString(msg))
		return 2
	}
	if th.Dead {
		msg := "can not resume a dead thread"
		if th.wrapped {
			L.RaiseError(msg)
			return 0
		}
		L.Push(LFalse)
		L.Push(LString(msg))
		return 2
	}
	th.Parent = L
	L.G.CurrentThread = th
	if !th.isStarted() {
		cf := th.stack.Last()
		th.currentFrame = cf
		th.SetTop(0)
		nargs := L.GetTop() - 1
		L.XMoveTo(th, nargs)
		cf.NArgs = int16(nargs)
		th.initCallFrame(cf)
		th.Panic = panicWithoutTraceback
	} else {
		nargs := L.GetTop() - 1
		L.XMoveTo(th, nargs)
	}
	top := L.GetTop()
	th.yieldState = yieldNone
	threadRun(th)
	if th.yieldState != yieldSystem || L.Parent == nil {
		return L.GetTop() - top
	}
	return coResumePropagate(L, th, top)
}

// coResumePropagate handles system yield propagation through a coroutine boundary.
// Called only when the inner thread yielded via a Go function returning -1
// (not coroutine.yield) and this coroutine has a parent to propagate to.
func coResumePropagate(L *LState, th *LState, top int) int {
	// switchToParentThread already moved yield values to L's stack.
	// For non-wrapped threads it also pushed LTrue before the values.
	// Extract the raw yield values.
	yieldStart := top + 1
	if !th.wrapped {
		yieldStart++ // skip LTrue from switchToParentThread
	}
	nvals := L.GetTop() - yieldStart + 1

	// Transfer yield values to parent thread. For non-wrapped outer coroutines,
	// Resume expects [LTrue, val1, val2, ...] on the parent's stack.
	parent := L.Parent
	if !L.wrapped {
		parent.Push(LTrue)
	}
	for i := 0; i < nvals; i++ {
		parent.Push(L.Get(yieldStart + i))
	}

	// Clear our stack so resume values land cleanly.
	L.SetTop(0)

	// Perform the thread switch manually to preserve the current frame.
	// Unlike switchToParentThread, we do NOT pop the frame — the continuation
	// installed below needs it to survive for the next resume.
	L.G.CurrentThread = parent
	L.Parent = nil
	L.yieldState = yieldSystem

	// Install continuation so the next resume re-enters the inner thread.
	ext := L.setFrameExt(L.currentFrame)
	ext.Continuation = coResumeContinuation
	ext.ContinuationCtx = th

	// callGFunction checks L.yieldState and skips switchToParentThread when set,
	// preserving the frame on the stack.
	return -1
}

// coResumeContinuation re-resumes the inner thread after a system yield was
// propagated through this coroutine boundary. Resume values are on L's stack.
func coResumeContinuation(L *LState, ctx interface{}, _ ResumeState) int {
	th := ctx.(*LState)

	th.Parent = L
	L.G.CurrentThread = th
	nargs := L.GetTop()
	L.XMoveTo(th, nargs)
	th.yieldState = yieldNone

	top := L.GetTop()
	threadRun(th)
	if th.yieldState != yieldSystem || L.Parent == nil {
		return L.GetTop() - top
	}
	return coResumePropagate(L, th, top)
}

func coRunning(L *LState) int {
	if L.G.MainThread == L {
		L.Push(LNil)
		return 1
	}
	L.Push(L.G.CurrentThread)
	return 1
}

func coStatus(L *LState) int {
	L.Push(LString(L.Status(L.CheckThread(1))))
	return 1
}

func wrapaux(L *LState) int {
	L.Insert(L.ToThread(UpvalueIndex(1)), 1)
	return coResume(L)
}

func coWrap(L *LState) int {
	coCreate(L)
	L.CheckThread(L.GetTop()).wrapped = true
	v := L.Get(L.GetTop())
	L.Pop(1)
	L.Push(L.NewClosure(wrapaux, v))
	return 1
}
