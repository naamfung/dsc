package lua

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

// CompileOptions configures compile-time type resolution for type calls.
//
// TypeInfo should contain the encoded manifest bytes. When provided, its type
// names are added to the compile-time type name set and stored on the proto.
// TypeNames can be used to supply additional compile-time type names directly.
type CompileOptions struct {
	TypeInfo  []byte
	TypeNames map[string]struct{}
}

// CompileWithOptions compiles Lua source with compile-time type call resolution.
func CompileWithOptions(chunk []ast.Stmt, name string, opts CompileOptions) (proto *FunctionProto, err error) {
	defer func() {
		if rcv := recover(); rcv != nil {
			if _, ok := rcv.(*CompileError); ok {
				err = rcv.(error)
			} else {
				panic(rcv)
			}
		}
	}()

	parlist := &ast.ParList{HasVargs: true, Names: []string{}}
	funcexpr := &ast.FunctionExpr{ParList: parlist, Stmts: chunk}
	if len(chunk) > 0 {
		funcexpr.SetLastLine(sline(chunk[0]))
		funcexpr.SetLastLine(eline(chunk[len(chunk)-1]) + 1)
	}

	typeNames := defaultCompileTypeNames(opts.TypeNames)
	for name := range collectTopLevelTypeNames(chunk) {
		typeNames[name] = struct{}{}
	}
	if len(opts.TypeInfo) > 0 {
		if manifest := safeDecodeManifest(opts.TypeInfo); manifest != nil {
			for name := range manifest.Types {
				if name != "" {
					typeNames[name] = struct{}{}
				}
			}
		}
	}

	context := newFuncContext(name, nil, typeNames)
	compileFunctionExpr(context, funcexpr, ecnone(0))
	proto = context.Proto
	if len(opts.TypeInfo) > 0 {
		proto.SetTypeInfo(opts.TypeInfo)
	}
	return
}

func defaultCompileTypeNames(extra map[string]struct{}) map[string]struct{} {
	names := map[string]struct{}{
		"nil":     {},
		"boolean": {},
		"number":  {},
		"integer": {},
		"string":  {},
		"any":     {},
		"unknown": {},
		"never":   {},
	}
	for name := range extra {
		if name != "" {
			names[name] = struct{}{}
		}
	}
	return names
}

func collectTopLevelTypeNames(chunk []ast.Stmt) map[string]struct{} {
	if len(chunk) == 0 {
		return nil
	}
	names := map[string]struct{}{}
	for _, stmt := range chunk {
		switch def := stmt.(type) {
		case *ast.TypeDefStmt:
			if def.Name != "" {
				names[def.Name] = struct{}{}
			}
		case *ast.InterfaceDefStmt:
			if def.Name != "" {
				names[def.Name] = struct{}{}
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}
