package typ

import (
	"reflect"
	"unsafe"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// TypeEquals compares two types for structural equality with cycle detection.
//
// Uses coinductive equality for recursive types: if the same type pair is
// encountered again during traversal, they are assumed equal. This handles
// infinite recursive structures correctly.
//
// Aliases are transparent: compares through to their targets.
func TypeEquals(a, b Type) bool {
	guard := NewGuard()
	return typeEqualsGuard(a, b, guard, nil)
}

func typeEqualsGuard(a, b Type, guard internal.RecursionGuard, seen map[typePair]bool) bool {
	a = unwrapAliasForEquals(a, guard)
	b = unwrapAliasForEquals(b, guard)

	if a == b {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	// Unwrap Ref types
	if ra, ok := a.(*Ref); ok {
		if rb, ok := b.(*Ref); ok {
			return ra.Module == rb.Module && ra.Name == rb.Name
		}

		return false
	}

	if _, ok := b.(*Ref); ok {
		return false
	}

	if a.Kind() != b.Kind() {
		return false
	}

	// For compound types, use pointer-based cycle detection to avoid hash collisions.
	if needsCycleCheck(a.Kind()) {
		ap := typePointer(a)
		bp := typePointer(b)

		if ap != 0 && bp != 0 {
			pair := typePair{a: ap, b: bp}

			if seen == nil {
				seen = make(map[typePair]bool)
			}

			if seen[pair] {
				return true // cycle, coinductively equal
			}

			seen[pair] = true
		}
	}

	next, ok := guard.Enter(a)
	if !ok {
		return false
	}

	a = unwrapTransparentWrappers(a)
	b = unwrapTransparentWrappers(b)

	switch va := a.(type) {
	case *Optional:
		vb, ok := b.(*Optional)
		return ok && typeEqualsGuard(va.Inner, vb.Inner, next, seen)
	case *Union:
		vb, ok := b.(*Union)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], next, seen) {
				return false
			}
		}
		return true
	case *Intersection:
		vb, ok := b.(*Intersection)
		if !ok || len(va.Members) != len(vb.Members) {
			return false
		}
		for i, m := range va.Members {
			if !typeEqualsGuard(m, vb.Members[i], next, seen) {
				return false
			}
		}
		return true
	case *Tuple:
		vb, ok := b.(*Tuple)
		if !ok || len(va.Elements) != len(vb.Elements) {
			return false
		}
		for i, e := range va.Elements {
			if !typeEqualsGuard(e, vb.Elements[i], next, seen) {
				return false
			}
		}
		return true
	case *Array:
		vb, ok := b.(*Array)
		return ok && typeEqualsGuard(va.Element, vb.Element, next, seen)
	case *Map:
		vb, ok := b.(*Map)
		return ok &&
			typeEqualsGuard(va.Key, vb.Key, next, seen) &&
			typeEqualsGuard(va.Value, vb.Value, next, seen)
	case *Record:
		vb, ok := b.(*Record)
		if !ok || va.Open != vb.Open || len(va.Fields) != len(vb.Fields) {
			return false
		}
		for i, f := range va.Fields {
			fb := vb.Fields[i]
			if f.Name != fb.Name || f.Optional != fb.Optional || f.Readonly != fb.Readonly {
				return false
			}
			if !typeEqualsGuard(f.Type, fb.Type, next, seen) {
				return false
			}
		}
		if va.HasMapComponent() != vb.HasMapComponent() {
			return false
		}
		if va.HasMapComponent() {
			if !typeEqualsGuard(va.MapKey, vb.MapKey, next, seen) {
				return false
			}
			if !typeEqualsGuard(va.MapValue, vb.MapValue, next, seen) {
				return false
			}
		}
		return true
	case *Function:
		vb, ok := b.(*Function)
		if !ok || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !typeEqualsGuard(tp, vb.TypeParams[i], next, seen) {
				return false
			}
		}
		if len(va.Params) != len(vb.Params) || len(va.Returns) != len(vb.Returns) {
			return false
		}
		for i, p := range va.Params {
			pb := vb.Params[i]
			if p.Optional != pb.Optional {
				return false
			}
			if !typeEqualsGuard(p.Type, pb.Type, next, seen) {
				return false
			}
		}
		if (va.Variadic == nil) != (vb.Variadic == nil) {
			return false
		}
		if va.Variadic != nil && !typeEqualsGuard(va.Variadic, vb.Variadic, next, seen) {
			return false
		}
		for i, r := range va.Returns {
			if !typeEqualsGuard(r, vb.Returns[i], next, seen) {
				return false
			}
		}
		return true
	case *Generic:
		vb, ok := b.(*Generic)
		if !ok || va.Name != vb.Name || len(va.TypeParams) != len(vb.TypeParams) {
			return false
		}
		for i, tp := range va.TypeParams {
			if !tp.Equals(vb.TypeParams[i]) {
				return false
			}
		}
		if va.Name != "" {
			return true
		}
		if (va.Body == nil) != (vb.Body == nil) {
			return false
		}
		return va.Body == nil || typeEqualsGuard(va.Body, vb.Body, next, seen)
	case *Instantiated:
		vb, ok := b.(*Instantiated)
		if !ok || !typeEqualsGuard(va.Generic, vb.Generic, next, seen) || len(va.TypeArgs) != len(vb.TypeArgs) {
			return false
		}
		for i, arg := range va.TypeArgs {
			if !typeEqualsGuard(arg, vb.TypeArgs[i], next, seen) {
				return false
			}
		}
		return true
	case *Recursive:
		vb, ok := b.(*Recursive)
		if !ok {
			return false
		}
		if va.ID == vb.ID {
			return true
		}
		if va.Name != vb.Name {
			return false
		}
		return typeEqualsGuard(va.Body, vb.Body, next, seen)
	default:
		return a.Equals(b)
	}
}

func unwrapAliasForEquals(t Type, guard internal.RecursionGuard) Type {
	for t != nil {
		t = unwrapTransparentWrappers(t)
		next, ok := guard.Enter(t)
		if !ok {
			return nil
		}
		guard = next

		alias, ok := t.(*Alias)
		if !ok {
			return t
		}
		t = alias.UnaliasedTarget()
	}
	return nil
}

func needsCycleCheck(k kind.Kind) bool {
	switch k {
	case kind.Union, kind.Intersection, kind.Record, kind.Function,
		kind.Generic, kind.Instantiated, kind.Interface, kind.Recursive:
		return true
	}

	return false
}

type typePair struct {
	a uintptr
	b uintptr
}

func typePointer(t Type) uintptr {
	switch tt := t.(type) {
	case *Union:
		return uintptr(unsafe.Pointer(tt))
	case *Intersection:
		return uintptr(unsafe.Pointer(tt))
	case *Record:
		return uintptr(unsafe.Pointer(tt))
	case *Function:
		return uintptr(unsafe.Pointer(tt))
	case *Generic:
		return uintptr(unsafe.Pointer(tt))
	case *Instantiated:
		return uintptr(unsafe.Pointer(tt))
	case *Interface:
		return uintptr(unsafe.Pointer(tt))
	case *Recursive:
		return uintptr(unsafe.Pointer(tt))
	}

	v := reflect.ValueOf(t)
	if v.Kind() != reflect.Pointer {
		return 0
	}

	return v.Pointer()
}

// TypeString returns string representation with depth limiting.
func TypeString(t Type) string {
	guard := NewGuard()
	return typeStringGuard(t, guard)
}

func typeStringGuard(t Type, guard internal.RecursionGuard) string {
	if t == nil {
		return "nil"
	}
	return WithGuard(t, guard, "...", func(internal.RecursionGuard) string {
		return t.String()
	})
}
