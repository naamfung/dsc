package lua

import (
	"io"
	"strings"

	"github.com/wippyai/go-lua/compiler/parse"
)

// CompileString compiles Lua source code to bytecode without requiring an LState.
// The returned FunctionProto can be reused across multiple LStates.
func CompileString(source, name string) (*FunctionProto, error) {
	return CompileReader(strings.NewReader(source), name)
}

// CompileReader compiles Lua source from a reader to bytecode.
func CompileReader(reader io.Reader, name string) (*FunctionProto, error) {
	chunk, err := parse.Parse(reader, name)
	if err != nil {
		return nil, err
	}
	return Compile(chunk, name)
}

// LoadProto creates an LFunction from a precompiled FunctionProto.
// This is much faster than Load() since it skips parsing and compilation.
func (ls *LState) LoadProto(proto *FunctionProto) *LFunction {
	ls.injectProtoTypes(proto)
	return newLFunctionL(proto, ls.currentEnv(), 0)
}
