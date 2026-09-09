package lua

import (
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

type typeBinding struct {
	name  string
	value LValue
}

type typeResolver struct {
	path  string
	types map[string]typ.Type
}

func (fp *FunctionProto) runtimeTypeBindings() []typeBinding {
	if fp == nil || len(fp.TypeInfo) == 0 {
		return nil
	}
	fp.typeInfoOnce.Do(func() {
		manifest := safeDecodeManifest(fp.TypeInfo)
		if manifest == nil || len(manifest.Types) == 0 {
			return
		}
		resolver := &typeResolver{path: manifest.Path, types: manifest.Types}
		bindings := make([]typeBinding, 0, len(manifest.Types))
		bindingsByName := make(map[string]*LType, len(manifest.Types))
		for name, t := range manifest.Types {
			if name == "" || t == nil {
				continue
			}
			lt := newRuntimeTypeValue(t, name, resolver)
			bindings = append(bindings, typeBinding{name: name, value: lt})
			bindingsByName[name] = lt
		}
		fp.typeBindings = bindings
		fp.typeBindingsByName = bindingsByName
	})
	return fp.typeBindings
}

func (fp *FunctionProto) runtimeTypeValueByName(name string) *LType {
	if name == "" {
		return nil
	}
	if builtin := runtimeBuiltinTypeByName(name); builtin != nil {
		return builtin
	}
	fp.runtimeTypeBindings()
	if fp.typeBindingsByName == nil {
		return nil
	}
	return fp.typeBindingsByName[name]
}

func safeDecodeManifest(data []byte) (manifest *typeio.Manifest) {
	if len(data) == 0 {
		return nil
	}
	defer func() {
		if recover() != nil {
			manifest = nil
		}
	}()
	decoded, err := typeio.DecodeManifest(data)
	if err != nil {
		return nil
	}
	return decoded
}

func newRuntimeTypeValue(t typ.Type, name string, resolver *typeResolver) *LType {
	return &LType{
		inner:    unwrapRuntimeAlias(t),
		name:     name,
		resolver: resolver,
	}
}

func unwrapRuntimeAlias(t typ.Type) typ.Type {
	for {
		if alias, ok := t.(*typ.Alias); ok && alias.Target != nil {
			t = alias.Target
			continue
		}
		return t
	}
}

func (ls *LState) injectProtoTypes(proto *FunctionProto) {
	if ls == nil || proto == nil {
		return
	}
	bindings := proto.runtimeTypeBindings()
	if len(bindings) == 0 {
		return
	}
	env := ls.Env
	if env == nil {
		return
	}
	for _, binding := range bindings {
		if binding.name == "" {
			continue
		}
		if isPrimitiveTypeName(binding.name) {
			continue
		}
		if env.RawGetString(binding.name) == LNil {
			env.RawSetString(binding.name, binding.value)
		}
	}
}

func runtimeBuiltinTypeByName(name string) *LType {
	switch name {
	case "nil":
		return LTypeNil
	case "boolean":
		return LTypeBoolean
	case "number":
		return LTypeNumber
	case "integer":
		return LTypeInteger
	case "string":
		return LTypeString
	case "any":
		return LTypeAny
	case "unknown":
		return LTypeUnknown
	case "never":
		return LTypeNever
	default:
		return nil
	}
}

func isPrimitiveTypeName(name string) bool {
	return runtimeBuiltinTypeByName(name) != nil
}

var typeMethodNames = map[string]struct{}{
	"is":       {},
	"kind":     {},
	"name":     {},
	"elem":     {},
	"key":      {},
	"val":      {},
	"inner":    {},
	"ret":      {},
	"fields":   {},
	"variants": {},
	"params":   {},
	"tparams":  {},
}
