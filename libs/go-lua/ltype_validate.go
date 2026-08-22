package lua

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/validate"
)

// ValidatorFunc validates an LValue against an annotation argument.
type ValidatorFunc func(val LValue, arg any) *validate.Error

// ValidationContext wraps validate.Registry for LValue validation.
type ValidationContext struct {
	registry *validate.Registry
}

// NewValidationContext creates empty context.
func NewValidationContext() *ValidationContext {
	return &ValidationContext{registry: validate.New()}
}

// DefaultValidationContext returns context with built-in validators.
func DefaultValidationContext() *ValidationContext {
	return &ValidationContext{registry: validate.Default}
}

// RegisterValidator adds a validator that works with LValue.
func (vc *ValidationContext) RegisterValidator(name string, fn ValidatorFunc) {
	vc.registry.RegisterValidator(name, func(val any, arg any) *validate.Error {
		lv, ok := val.(LValue)
		if !ok {
			return nil
		}
		return fn(lv, arg)
	})
}

// Validate checks value against type with annotations.
// Uses zero-alloc path building - paths only built on error.
func (vc *ValidationContext) Validate(val LValue, lt *LType) []*validate.Error {
	var errors []*validate.Error
	var path pathBuilder
	vc.validateValue(val, lt.inner, &path, &errors)
	return errors
}

// pathBuilder builds field paths lazily to avoid allocations in happy path.
type pathBuilder struct {
	segments []string
}

func (p *pathBuilder) push(s string) {
	p.segments = append(p.segments, s)
}

func (p *pathBuilder) pushIndex(i int) {
	p.segments = append(p.segments, "["+strconv.Itoa(i)+"]")
}

func (p *pathBuilder) pop() {
	if len(p.segments) > 0 {
		p.segments = p.segments[:len(p.segments)-1]
	}
}

func (p *pathBuilder) String() string {
	if len(p.segments) == 0 {
		return ""
	}
	var b strings.Builder
	for i, s := range p.segments {
		if i > 0 && s[0] != '[' {
			b.WriteByte('.')
		}
		b.WriteString(s)
	}
	return b.String()
}

func (vc *ValidationContext) validateValue(val LValue, t typ.Type, path *pathBuilder, errors *[]*validate.Error) {
	// Handle annotated types: unwrap and check annotations
	if ann, ok := t.(*typ.Annotated); ok {
		vc.validateValue(val, ann.Inner, path, errors)
		vc.checkAnnotations(val, ann.Annotations, path, errors)
		return
	}

	if !validateBasic(val, t) {
		*errors = append(*errors, &validate.Error{Field: path.String(), Message: "type mismatch", Got: val})
		return
	}

	switch tt := t.(type) {
	case *typ.Record:
		tbl := val.(*LTable)
		for _, field := range tt.Fields {
			fv := LNil
			if tbl.Strdict != nil {
				if v := tbl.Strdict[field.Name]; v != nil {
					fv = v
				}
			}
			path.push(field.Name)
			if fv == LNil {
				if !field.Optional && !validateBasic(LNil, field.Type) {
					*errors = append(*errors, &validate.Error{Field: path.String(), Message: "missing required field"})
				}
			} else {
				vc.validateValue(fv, field.Type, path, errors)
			}
			path.pop()
		}

	case *typ.Array:
		tbl := val.(*LTable)
		for i, elem := range tbl.Array {
			if elem == LNil {
				continue
			}
			path.pushIndex(i)
			vc.validateValue(elem, tt.Element, path, errors)
			path.pop()
		}

	case *typ.Map:
		tbl := val.(*LTable)
		for k, v := range tbl.Dict {
			path.push("[key]")
			vc.validateValue(k, tt.Key, path, errors)
			path.pop()
			path.push("[value]")
			vc.validateValue(v, tt.Value, path, errors)
			path.pop()
		}
		for k, v := range tbl.Strdict {
			path.push(k)
			vc.validateValue(v, tt.Value, path, errors)
			path.pop()
		}

	case *typ.Optional:
		if val != LNil {
			vc.validateValue(val, tt.Inner, path, errors)
		}

	case *typ.Union:
		for _, variant := range tt.Members {
			if validateBasic(val, variant) {
				vc.validateValue(val, variant, path, errors)
				return
			}
		}

	case *typ.Intersection:
		for _, member := range tt.Members {
			vc.validateValue(val, member, path, errors)
		}

	case *typ.Recursive:
		if tt.Body != nil {
			vc.validateValue(val, tt.Body, path, errors)
		}

	case *typ.Tuple:
		tbl := val.(*LTable)
		for i, elemType := range tt.Elements {
			var elemVal LValue
			if i < len(tbl.Array) {
				elemVal = tbl.Array[i]
			} else {
				elemVal = LNil
			}
			path.pushIndex(i + 1)
			vc.validateValue(elemVal, elemType, path, errors)
			path.pop()
		}
	}
}

func (vc *ValidationContext) checkAnnotations(val LValue, annotations []typ.Annotation, path *pathBuilder, errors *[]*validate.Error) {
	for _, ann := range annotations {
		if fn := vc.registry.Get(ann.Name); fn != nil {
			if err := fn(val, ann.Arg); err != nil {
				err.Field = path.String()
				*errors = append(*errors, err)
			}
		}
	}
}

func validateBasic(val LValue, t typ.Type) bool {
	switch t.Kind() {
	case kind.Nil:
		return val == LNil
	case kind.Boolean:
		_, ok := val.(LBool)
		return ok
	case kind.Number:
		switch val.(type) {
		case LNumber, LInteger:
			return true
		}
		return false
	case kind.Integer:
		if _, ok := val.(LInteger); ok {
			return true
		}
		if n, ok := val.(LNumber); ok {
			return IsIntegerValue(n)
		}
		return false
	case kind.String:
		_, ok := val.(LString)
		return ok
	case kind.Any, kind.Unknown:
		return true
	case kind.Never:
		return false
	case kind.Function:
		switch val.(type) {
		case *LFunction, LGoFunc:
			return true
		}
		return false
	case kind.Array, kind.Map, kind.Record, kind.Tuple,
		kind.Sum, kind.Interface, kind.Intersection:
		_, ok := val.(*LTable)
		return ok
	case kind.Recursive:
		if rec, ok := t.(*typ.Recursive); ok && rec.Body != nil {
			return validateBasic(val, rec.Body)
		}
		return true
	case kind.Platform:
		_, ok := val.(*LUserData)
		return ok
	case kind.Generic, kind.TypeParam, kind.Self, kind.Meta:
		return true
	default:
		// fall though
	}

	switch tt := t.(type) {
	case *typ.Annotated:
		return validateBasic(val, tt.Inner)
	case *typ.Optional:
		return val == LNil || validateBasic(val, tt.Inner)
	case *typ.Union:
		for _, ut := range tt.Members {
			if validateBasic(val, ut) {
				return true
			}
		}
		return false
	case *typ.Array, *typ.Map, *typ.Record, *typ.Tuple:
		_, ok := val.(*LTable)
		return ok
	case *typ.Function:
		switch val.(type) {
		case *LFunction, LGoFunc:
			return true
		}
		return false
	case *typ.Literal:
		return validateLiteralTyp(val, tt)
	case *typ.Alias:
		return validateBasic(val, tt.Target)
	case *typ.Ref:
		// Unresolved reference - assume valid
		return true
	case *typ.Instantiated:
		if tt.Generic != nil && tt.Generic.Body != nil {
			return validateBasic(val, tt.Generic.Body)
		}
		return true
	}
	return false
}

func validateLiteralTyp(val LValue, lt *typ.Literal) bool {
	switch lit := lt.Value.(type) {
	case string:
		s, ok := val.(LString)
		return ok && string(s) == lit
	case float64:
		n, ok := val.(LNumber)
		return ok && float64(n) == lit
	case int64:
		if i, ok := val.(LInteger); ok {
			return int64(i) == lit
		}
		if n, ok := val.(LNumber); ok {
			return float64(n) == float64(lit)
		}
	case bool:
		b, ok := val.(LBool)
		return ok && bool(b) == lit
	}
	return false
}

// Make LNumber implement float64er for validate package
func (nm LNumber) Float64() float64 { return float64(nm) }

// Make LInteger implement int64er for validate package
func (i LInteger) Int64() int64 { return int64(i) }

// Make LString implement stringer for validate package
// (already has String() method)

// Make LBool implement booler for validate package
func (bl LBool) Bool() bool { return bool(bl) }

// LTable already has Len() method in table.go
