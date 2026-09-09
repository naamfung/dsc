package lua

const (
	// BaseLibName is here for consistency; the base functions have no namespace/library.
	BaseLibName = ""
	// LoadLibName is here for consistency; the loading system has no namespace/library.
	LoadLibName = "package"
	// TabLibName is the name of the table Library.
	TabLibName = "table"
	// StringLibName is the name of the string Library.
	StringLibName = "string"
	// MathLibName is the name of the math Library.
	MathLibName = "math"
	// DebugLibName is the name of the debug Library.
	DebugLibName = "debug"
	// CoroutineLibName is the name of the coroutine Library.
	CoroutineLibName = "coroutine"
	// Utf8LibName is the name of the utf8 Library.
	Utf8LibName = "utf8"
	// ErrorsLibName is the name of the errors Library.
	ErrorsLibName = "errors"
)

type luaLib struct {
	libName string
	libFunc LGoFunc
}

var luaLibs = []luaLib{
	{LoadLibName, OpenPackage},
	{BaseLibName, OpenBase},
	{TabLibName, OpenTable},
	{StringLibName, OpenString},
	{MathLibName, OpenMath},
	{DebugLibName, OpenDebug},
	{CoroutineLibName, OpenCoroutine},
	{Utf8LibName, OpenUtf8},
	{ErrorsLibName, OpenErrors},
}

// OpenLibs loads the built-in libraries. It is equivalent to running OpenLoad,
// then OpenBase, then iterating over the other OpenXXX functions in any order.
func (ls *LState) OpenLibs() {
	// NB: Map iteration order in Go is deliberately randomised, so must open Load/Base
	// prior to iterating.
	for _, lib := range luaLibs {
		ls.Push(lib.libFunc)
		ls.Push(LString(lib.libName))
		ls.Call(1, 0)
	}
}
