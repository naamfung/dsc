package lua

import (
	"fmt"
	"strconv"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/validate"
)

// LType is a runtime type value that can be used in Lua code.
// It wraps a typ.Type and implements the LValue interface.
// Types are first-class values: callable for validation, comparable,
// and support reflection via direct VM dispatch (no metatables).
type LType struct {
	inner typ.Type
	name  string // optional name for named types
	// resolver provides local type resolution for refs within manifest-defined types.
	resolver *typeResolver
}

func (lt *LType) String() string {
	if lt.name != "" {
		return lt.name
	}
	if lt.inner == nil {
		return "unknown"
	}
	return lt.inner.String()
}

func (lt *LType) Type() LValueType { return LTType }

// Inner returns the underlying typ.Type.
func (lt *LType) Inner() typ.Type { return lt.inner }

// Name returns the type name, or empty string for anonymous types.
func (lt *LType) Name() string { return lt.name }

// NewLType creates a new LType wrapping the given type.
func NewLType(t typ.Type) *LType {
	return &LType{inner: t}
}

// NewNamedLType creates a new named LType.
func NewNamedLType(t typ.Type, name string) *LType {
	return &LType{inner: t, name: name}
}

// Primitive type singletons - zero allocation for common types
var (
	LTypeNil     = &LType{inner: typ.Nil, name: "nil"}
	LTypeBoolean = &LType{inner: typ.Boolean, name: "boolean"}
	LTypeNumber  = &LType{inner: typ.Number, name: "number"}
	LTypeInteger = &LType{inner: typ.Integer, name: "integer"}
	LTypeString  = &LType{inner: typ.String, name: "string"}
	LTypeAny     = &LType{inner: typ.Any, name: "any"}
	LTypeUnknown = &LType{inner: typ.Unknown, name: "unknown"}
	LTypeNever   = &LType{inner: typ.Never, name: "never"}
)

// KindString returns a string representation of the type's kind.
func (lt *LType) KindString() string {
	if lt.inner == nil {
		return "unknown"
	}
	switch lt.inner.Kind() {
	case kind.Nil:
		return "nil"
	case kind.Boolean:
		return "boolean"
	case kind.Number:
		return "number"
	case kind.Integer:
		return "integer"
	case kind.String:
		return "string"
	case kind.Any:
		return "any"
	case kind.Unknown:
		return "unknown"
	case kind.Never:
		return "never"
	case kind.Optional:
		return "optional"
	case kind.Union:
		return "union"
	case kind.Intersection:
		return "intersection"
	case kind.Tuple:
		return "tuple"
	case kind.Function:
		return "function"
	case kind.Array:
		return "array"
	case kind.Map:
		return "map"
	case kind.Record:
		return "record"
	case kind.Generic:
		return "generic"
	case kind.Instantiated:
		return "instantiated"
	default:
		return "unknown"
	}
}

// Validate checks if a value matches this type.
// Returns true if valid, false otherwise.
func (lt *LType) Validate(L *LState, val LValue) bool {
	return validateValue(val, lt.inner, lt.resolver)
}

// builtinRuntimeTypes maps well-known type names to their runtime type
// representations. Used as a fallback when the module's type resolver
// doesn't contain the referenced type (builtins are not in manifests).
var builtinRuntimeTypes = map[string]typ.Type{
	"nil":     typ.Nil,
	"boolean": typ.Boolean,
	"number":  typ.Number,
	"integer": typ.Integer,
	"string":  typ.String,
	"any":     typ.Any,
	"unknown": typ.Unknown,
	"never":   typ.Never,
	"table":   typ.NewInterface("table", nil),
}

func resolveRuntimeType(t typ.Type, resolver *typeResolver, depth int) typ.Type {
	if t == nil {
		return nil
	}
	// Fast path: most types are not Alias or Ref
	k := t.Kind()
	if k != kind.Alias && k != kind.Ref {
		return t
	}
	for t != nil && depth < 128 {
		switch tt := t.(type) {
		case *typ.Alias:
			if tt.Target == nil {
				return t
			}
			t = tt.Target
		case *typ.Ref:
			if resolver != nil && (tt.Module == "" || tt.Module == resolver.path) {
				if target, ok := resolver.types[tt.Name]; ok && target != nil {
					t = target
					break
				}
			}
			// Fallback to builtin types for well-known names
			if tt.Module == "" {
				if builtin, ok := builtinRuntimeTypes[tt.Name]; ok {
					t = builtin
					break
				}
			}
			return t
		default:
			return t
		}
		depth++
	}
	return t
}

// maxValidationDepth limits recursion for Recursive types and deeply nested structures.
const maxValidationDepth = 64

func validateValue(val LValue, t typ.Type, resolver *typeResolver) bool {
	return validateValueDepth(val, t, resolver, 0)
}

func validateValueDepth(val LValue, t typ.Type, resolver *typeResolver, depth int) bool {
	if depth > maxValidationDepth {
		return false
	}
	if t == nil {
		return false
	}
	t = resolveRuntimeType(t, resolver, 0)
	if t == nil {
		return false
	}
	if ann, ok := t.(*typ.Annotated); ok {
		if ann.Inner == nil {
			return false
		}
		if !validateValueDepth(val, ann.Inner, resolver, depth+1) {
			return false
		}
		return validateAnnotations(val, ann.Annotations, "") == nil
	}
	// Handle primitives by Kind (they're value types, not pointers)
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
		_, ok := val.(LInteger)
		if ok {
			return true
		}
		// Also accept LNumber if it's an integer value
		if n, ok := val.(LNumber); ok {
			return IsIntegerValue(n)
		}
		return false

	case kind.String:
		_, ok := val.(LString)
		return ok

	case kind.Any, kind.Unknown, kind.Self:
		// Self is a compile-time placeholder; at runtime treat as permissive
		return true

	case kind.Never:
		return false
	}

	// Handle compound types (pointers)
	switch tt := t.(type) {
	case *typ.Optional:
		if val == LNil {
			return true
		}
		return validateValueDepth(val, tt.Inner, resolver, depth+1)

	case *typ.Union:
		for _, ut := range tt.Members {
			if validateValueDepth(val, ut, resolver, depth+1) {
				return true
			}
		}
		return false

	case *typ.Array:
		tbl, ok := val.(*LTable)
		if !ok {
			return false
		}
		// Check array part
		for _, v := range tbl.Array {
			if v != LNil && !validateValueDepth(v, tt.Element, resolver, depth+1) {
				return false
			}
		}
		return true

	case *typ.Map:
		tbl, ok := val.(*LTable)
		if !ok {
			return false
		}
		if tt.Key == nil || tt.Value == nil {
			return false
		}
		for k, v := range tbl.Dict {
			if !validateValueDepth(k, tt.Key, resolver, depth+1) {
				return false
			}
			if !validateValueDepth(v, tt.Value, resolver, depth+1) {
				return false
			}
		}
		if tt.Key != nil && tt.Key.Kind() == kind.String {
			for _, v := range tbl.Strdict {
				if !validateValueDepth(v, tt.Value, resolver, depth+1) {
					return false
				}
			}
		}
		keyIsNum := tt.Key != nil && (tt.Key.Kind() == kind.Number || tt.Key.Kind() == kind.Integer || tt.Key.Kind() == kind.Any)
		if keyIsNum && len(tbl.Array) > 0 {
			for _, v := range tbl.Array {
				if v != LNil && !validateValueDepth(v, tt.Value, resolver, depth+1) {
					return false
				}
			}
		}
		return true

	case *typ.Record:
		tbl, ok := val.(*LTable)
		if !ok {
			return false
		}
		for _, field := range tt.Fields {
			var fieldVal LValue
			if tbl.Strdict != nil {
				fieldVal = tbl.Strdict[field.Name]
			}
			if fieldVal == nil {
				fieldVal = LNil
			}
			if fieldVal == LNil && field.Optional {
				continue
			}
			if !validateValueDepth(fieldVal, field.Type, resolver, depth+1) {
				return false
			}
		}
		if tt.HasMapComponent() {
			for k, v := range tbl.Dict {
				if !validateValueDepth(k, tt.MapKey, resolver, depth+1) {
					return false
				}
				if !validateValueDepth(v, tt.MapValue, resolver, depth+1) {
					return false
				}
			}
			if tt.MapKey.Kind() == kind.String {
				for k, v := range tbl.Strdict {
					isField := false
					for _, f := range tt.Fields {
						if f.Name == k {
							isField = true
							break
						}
					}
					if isField {
						continue
					}
					if !validateValueDepth(v, tt.MapValue, resolver, depth+1) {
						return false
					}
				}
			}
		}
		return true

	case *typ.Function:
		switch val.(type) {
		case *LFunction, LGoFunc:
			return true
		}
		return false

	case *typ.Tuple:
		// Tuples are represented as tables with numeric keys
		tbl, ok := val.(*LTable)
		if !ok {
			return false
		}
		for i, elemType := range tt.Elements {
			var elemVal LValue
			if i < len(tbl.Array) {
				elemVal = tbl.Array[i]
			} else {
				elemVal = LNil
			}
			if !validateValueDepth(elemVal, elemType, resolver, depth+1) {
				return false
			}
		}
		return true

	case *typ.Literal:
		switch lit := tt.Value.(type) {
		case string:
			if s, ok := val.(LString); ok {
				return string(s) == lit
			}
		case float64:
			if n, ok := val.(LNumber); ok {
				return float64(n) == lit
			}
		case int64:
			if i, ok := val.(LInteger); ok {
				return int64(i) == lit
			}
			if n, ok := val.(LNumber); ok {
				return float64(n) == float64(lit)
			}
		case bool:
			if b, ok := val.(LBool); ok {
				return bool(b) == lit
			}
		}
		return false

	case *typ.Interface:
		_, ok := val.(*LTable)
		return ok

	case *typ.Intersection:
		for _, member := range tt.Members {
			if !validateValueDepth(val, member, resolver, depth+1) {
				return false
			}
		}
		return true

	case *typ.Recursive:
		if tt.Body == nil {
			return false
		}
		return validateValueDepth(val, tt.Body, resolver, depth+1)

	case *typ.Sum:
		// Sum types are tagged unions. At runtime, a value is a table
		// with a discriminant field. Accept any table.
		_, ok := val.(*LTable)
		return ok

	case *typ.Platform:
		// Platform types represent opaque host types (userdata).
		_, ok := val.(*LUserData)
		return ok

	case *typ.Generic:
		return false

	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded != nil && expanded != tt {
			return validateValueDepth(val, expanded, resolver, depth+1)
		}
		return false

	case *typ.Ref:
		return false
	}

	return false
}

// validationError holds structured validation failure information.
type validationError struct {
	Field      string // dot-separated field path (empty for root)
	Expected   string // expected type description
	Got        string // actual type description
	Message    string // human-readable error
	Constraint string // annotation constraint name (empty for type mismatches)
}

func (e *validationError) String() string {
	return e.Message
}

func newTypeError(path, expected, got string) *validationError {
	msg := "expected " + expected + ", got " + got
	if path != "" {
		msg = path + ": " + msg
	}
	return &validationError{
		Field:    path,
		Expected: expected,
		Got:      got,
		Message:  msg,
	}
}

func newMissingFieldError(path, fieldName, expected string) *validationError {
	fieldPath := fieldName
	if path != "" {
		fieldPath = path + "." + fieldName
	}
	msg := fieldPath + ": required field missing"
	return &validationError{
		Field:    fieldPath,
		Expected: expected,
		Got:      "nil",
		Message:  msg,
	}
}

func newConstraintError(path, constraint, message string, got, expected string) *validationError {
	msg := message
	if path != "" {
		msg = path + ": " + msg
	}
	return &validationError{
		Field:      path,
		Expected:   expected,
		Got:        got,
		Message:    msg,
		Constraint: constraint,
	}
}

func newUnresolvedError(path, typeName string) *validationError {
	msg := "unresolved type " + typeName
	if path != "" {
		msg = path + ": " + msg
	}
	return &validationError{
		Field:    path,
		Expected: typeName,
		Message:  msg,
	}
}

func validateWithError(val LValue, t typ.Type, resolver *typeResolver, path string) (bool, *validationError) {
	return validateWithErrorDepth(val, t, resolver, path, 0)
}

func validateWithErrorDepth(val LValue, t typ.Type, resolver *typeResolver, path string, depth int) (bool, *validationError) {
	if depth > maxValidationDepth {
		return false, newTypeError(path, "type", "recursion limit exceeded")
	}
	if t == nil {
		return false, newTypeError(path, "unknown", luaTypeName(val))
	}
	t = resolveRuntimeType(t, resolver, 0)
	if t == nil {
		return false, newTypeError(path, "unknown", luaTypeName(val))
	}
	if ann, ok := t.(*typ.Annotated); ok {
		if ann.Inner == nil {
			return false, newTypeError(path, "unknown", luaTypeName(val))
		}
		if ok, verr := validateWithErrorDepth(val, ann.Inner, resolver, path, depth+1); !ok {
			return false, verr
		}
		if annErr := validateAnnotations(val, ann.Annotations, path); annErr != nil {
			p := path
			if annErr.Field != "" {
				p = annErr.Field
			}
			got := ""
			if annErr.Got != nil {
				got = fmt.Sprint(annErr.Got)
			}
			expected := ""
			if annErr.Expected != nil {
				expected = fmt.Sprint(annErr.Expected)
			}
			return false, newConstraintError(p, annErr.Constraint, annErr.Message, got, expected)
		}
		return true, nil
	}
	switch t.Kind() {
	case kind.Nil:
		if val == LNil {
			return true, nil
		}
		return false, newTypeError(path, "nil", luaTypeName(val))

	case kind.Boolean:
		if _, ok := val.(LBool); ok {
			return true, nil
		}
		return false, newTypeError(path, "boolean", luaTypeName(val))

	case kind.Number:
		switch val.(type) {
		case LNumber, LInteger:
			return true, nil
		}
		return false, newTypeError(path, "number", luaTypeName(val))

	case kind.Integer:
		if _, ok := val.(LInteger); ok {
			return true, nil
		}
		if n, ok := val.(LNumber); ok && IsIntegerValue(n) {
			return true, nil
		}
		return false, newTypeError(path, "integer", luaTypeName(val))

	case kind.String:
		if _, ok := val.(LString); ok {
			return true, nil
		}
		return false, newTypeError(path, "string", luaTypeName(val))

	case kind.Any, kind.Unknown, kind.Self:
		return true, nil

	case kind.Never:
		return false, newTypeError(path, "never", luaTypeName(val))
	}

	typeName := t.String()

	switch tt := t.(type) {
	case *typ.Optional:
		if val == LNil {
			return true, nil
		}
		return validateWithErrorDepth(val, tt.Inner, resolver, path, depth+1)

	case *typ.Union:
		for _, ut := range tt.Members {
			if ok, _ := validateWithErrorDepth(val, ut, resolver, path, depth+1); ok {
				return true, nil
			}
		}
		return false, newTypeError(path, typeName, luaTypeName(val))

	case *typ.Array:
		tbl, ok := val.(*LTable)
		if !ok {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		for i, v := range tbl.Array {
			if v != LNil {
				elemPath := formatPath(path, i+1)
				if ok, verr := validateWithErrorDepth(v, tt.Element, resolver, elemPath, depth+1); !ok {
					return false, verr
				}
			}
		}
		return true, nil

	case *typ.Map:
		tbl, ok := val.(*LTable)
		if !ok {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		if tt.Key == nil || tt.Value == nil {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		for k, v := range tbl.Dict {
			if ok, _ := validateWithErrorDepth(k, tt.Key, resolver, path+"[key]", depth+1); !ok {
				return false, newTypeError(path+"[key]", tt.Key.String(), luaTypeName(k))
			}
			keyPath := formatPath(path, k)
			if ok, verr := validateWithErrorDepth(v, tt.Value, resolver, keyPath, depth+1); !ok {
				return false, verr
			}
		}
		if tt.Key != nil && tt.Key.Kind() == kind.String {
			for k, v := range tbl.Strdict {
				keyPath := path + "." + k
				if ok, verr := validateWithErrorDepth(v, tt.Value, resolver, keyPath, depth+1); !ok {
					return false, verr
				}
			}
		}
		keyIsNum := tt.Key != nil && (tt.Key.Kind() == kind.Number || tt.Key.Kind() == kind.Integer || tt.Key.Kind() == kind.Any)
		if keyIsNum && len(tbl.Array) > 0 {
			for i, v := range tbl.Array {
				if v != LNil {
					elemPath := formatPath(path, i+1)
					if ok, verr := validateWithErrorDepth(v, tt.Value, resolver, elemPath, depth+1); !ok {
						return false, verr
					}
				}
			}
		}
		return true, nil

	case *typ.Record:
		tbl, ok := val.(*LTable)
		if !ok {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		for _, field := range tt.Fields {
			var fieldVal LValue
			if tbl.Strdict != nil {
				fieldVal = tbl.Strdict[field.Name]
			}
			if fieldVal == nil {
				fieldVal = LNil
			}
			if fieldVal == LNil && field.Optional {
				continue
			}
			fieldPath := path + "." + field.Name
			if path == "" {
				fieldPath = field.Name
			}
			if fieldVal == LNil {
				ft := "unknown"
				if field.Type != nil {
					ft = field.Type.String()
				}
				return false, newMissingFieldError(path, field.Name, ft)
			}
			if ok, verr := validateWithErrorDepth(fieldVal, field.Type, resolver, fieldPath, depth+1); !ok {
				return false, verr
			}
		}
		if tt.HasMapComponent() {
			for k, v := range tbl.Dict {
				if ok, _ := validateWithErrorDepth(k, tt.MapKey, resolver, path+"[key]", depth+1); !ok {
					return false, newTypeError(path+"[key]", tt.MapKey.String(), luaTypeName(k))
				}
				keyPath := formatPath(path, k)
				if ok, verr := validateWithErrorDepth(v, tt.MapValue, resolver, keyPath, depth+1); !ok {
					return false, verr
				}
			}
			if tt.MapKey.Kind() == kind.String {
				for k, v := range tbl.Strdict {
					// Skip known record fields
					isField := false
					for _, f := range tt.Fields {
						if f.Name == k {
							isField = true
							break
						}
					}
					if isField {
						continue
					}
					keyPath := path + "." + k
					if ok, verr := validateWithErrorDepth(v, tt.MapValue, resolver, keyPath, depth+1); !ok {
						return false, verr
					}
				}
			}
		}
		return true, nil

	case *typ.Function:
		switch val.(type) {
		case *LFunction, LGoFunc:
			return true, nil
		}
		return false, newTypeError(path, "function", luaTypeName(val))

	case *typ.Tuple:
		tbl, ok := val.(*LTable)
		if !ok {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		for i, elemType := range tt.Elements {
			var elemVal LValue
			if i < len(tbl.Array) {
				elemVal = tbl.Array[i]
			} else {
				elemVal = LNil
			}
			elemPath := formatPath(path, i+1)
			if ok, verr := validateWithErrorDepth(elemVal, elemType, resolver, elemPath, depth+1); !ok {
				return false, verr
			}
		}
		return true, nil

	case *typ.Literal:
		switch lit := tt.Value.(type) {
		case string:
			if s, ok := val.(LString); ok && string(s) == lit {
				return true, nil
			}
		case float64:
			if n, ok := val.(LNumber); ok && float64(n) == lit {
				return true, nil
			}
		case int64:
			if i, ok := val.(LInteger); ok && int64(i) == lit {
				return true, nil
			}
			if n, ok := val.(LNumber); ok && float64(n) == float64(lit) {
				return true, nil
			}
		case bool:
			if b, ok := val.(LBool); ok && bool(b) == lit {
				return true, nil
			}
		}
		return false, newTypeError(path, t.String(), luaTypeName(val))

	case *typ.Interface:
		if _, ok := val.(*LTable); ok {
			return true, nil
		}
		return false, newTypeError(path, "table", luaTypeName(val))

	case *typ.Intersection:
		for _, member := range tt.Members {
			if ok, verr := validateWithErrorDepth(val, member, resolver, path, depth+1); !ok {
				return false, verr
			}
		}
		return true, nil

	case *typ.Recursive:
		if tt.Body == nil {
			return false, newTypeError(path, typeName, luaTypeName(val))
		}
		return validateWithErrorDepth(val, tt.Body, resolver, path, depth+1)

	case *typ.Sum:
		if _, ok := val.(*LTable); ok {
			return true, nil
		}
		return false, newTypeError(path, typeName, luaTypeName(val))

	case *typ.Platform:
		if _, ok := val.(*LUserData); ok {
			return true, nil
		}
		return false, newTypeError(path, typeName, luaTypeName(val))

	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded != nil && expanded != tt {
			return validateWithErrorDepth(val, expanded, resolver, path, depth+1)
		}
		return false, newTypeError(path, typeName, luaTypeName(val))

	case *typ.Ref:
		return false, newUnresolvedError(path, tt.Name)
	}

	return false, newTypeError(path, typeName, luaTypeName(val))
}

// toValidationLuaError converts a validationError into the proper *Error
// type with Kind=Invalid, structured details, and error metatable.
func toValidationLuaError(L *LState, verr *validationError) *Error {
	details := make(map[string]any, 4)
	if verr.Field != "" {
		details["field"] = verr.Field
	}
	if verr.Expected != "" {
		details["expected"] = verr.Expected
	}
	if verr.Got != "" {
		details["got"] = verr.Got
	}
	if verr.Constraint != "" {
		details["constraint"] = verr.Constraint
	}
	e := NewError(verr.Message).WithKind(Invalid).WithDetails(details)
	SetErrorMetatable(L, e)
	return e
}

func validateAnnotations(val LValue, annotations []typ.Annotation, path string) *validate.Error {
	if len(annotations) == 0 {
		return nil
	}
	for _, ann := range annotations {
		if fn := validate.Default.Get(ann.Name); fn != nil {
			if err := fn(val, ann.Arg); err != nil {
				if path != "" && err.Field == "" {
					err.Field = path
				}
				return err
			}
		}
	}
	return nil
}

func formatPath(base string, key interface{}) string {
	switch k := key.(type) {
	case int:
		if base == "" {
			return "[" + itoa(k) + "]"
		}
		return base + "[" + itoa(k) + "]"
	case LValue:
		s := k.String()
		if base == "" {
			return "[" + s + "]"
		}
		return base + "[" + s + "]"
	default:
		return base
	}
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return strconv.Itoa(i)
}

func luaTypeName(val LValue) string {
	if val == nil || val == LNil {
		return "nil"
	}
	switch val.(type) {
	case LBool:
		return "boolean"
	case LNumber, LInteger:
		return "number"
	case LString:
		return "string"
	case *LTable:
		return "table"
	case *LFunction, LGoFunc:
		return "function"
	case *LUserData:
		return "userdata"
	case *LType:
		return "type"
	default:
		return val.Type().String()
	}
}

// typeGetField handles field/method access on types.
// This is called from the VM for OP_GETTABLE on LType values.
// Methods take precedence to keep Type:is and related helpers reachable.
func (ls *LState) typeGetField(lt *LType, key string) LValue {
	// Methods
	switch key {
	case "is":
		return ls.createTypeMethod(lt, typeMethodIs)
	case "kind":
		return ls.createTypeMethod(lt, typeMethodKind)
	case "name":
		return ls.createTypeMethod(lt, typeMethodName)
	case "elem":
		return ls.createTypeMethod(lt, typeMethodElem)
	case "key":
		return ls.createTypeMethod(lt, typeMethodKey)
	case "val":
		return ls.createTypeMethod(lt, typeMethodVal)
	case "inner":
		return ls.createTypeMethod(lt, typeMethodInner)
	case "ret":
		return ls.createTypeMethod(lt, typeMethodRet)
	case "fields":
		return ls.createTypeMethod(lt, typeMethodFields)
	case "variants":
		return ls.createTypeMethod(lt, typeMethodVariants)
	case "params":
		return ls.createTypeMethod(lt, typeMethodParams)
	case "tparams":
		return ls.createTypeMethod(lt, typeMethodTparams)
	}

	// Record field access: Point.x returns type of field x
	if rec, ok := lt.inner.(*typ.Record); ok {
		for _, f := range rec.Fields {
			if f.Name == key {
				if f.Type == nil {
					return LNil
				}
				return &LType{inner: f.Type}
			}
		}
	}

	// Primitive type library methods: string.upper, string.format, etc.
	// Delegate to the builtin metatable for the corresponding Lua type.
	if lt == LTypeString {
		if mt := ls.G.builtinMts[int(LTString)]; mt != nil {
			if tbl, ok := mt.(*LTable); ok {
				return tbl.RawGetString(key)
			}
		}
	}

	return LNil
}

// Type method implementations
type typeMethodFunc func(*LState, *LType) int

func (ls *LState) createTypeMethod(lt *LType, method typeMethodFunc) LValue {
	// Return a closure that captures the type
	return LGoFunc(func(L *LState) int {
		return method(L, lt)
	})
}

func typeMethodIs(L *LState, lt *LType) int {
	// Support both colon syntax Type:is(val) and dot syntax Type.is(val)
	idx := 1
	if L.GetTop() >= 2 {
		idx = 2
	}
	val := L.Get(idx)
	// Fast path: zero-alloc boolean check handles the common success case.
	// Error details are only computed on failure.
	if validateValue(val, lt.inner, lt.resolver) {
		L.Push(val)
		L.Push(LNil)
	} else {
		_, verr := validateWithError(val, lt.inner, lt.resolver, "")
		L.Push(LNil)
		if verr != nil {
			L.Push(toValidationLuaError(L, verr))
		} else {
			L.Push(toValidationLuaError(L, newTypeError("", lt.String(), luaTypeName(val))))
		}
	}
	return 2
}

func typeMethodKind(L *LState, lt *LType) int {
	L.Push(LString(lt.KindString()))
	return 1
}

func typeMethodName(L *LState, lt *LType) int {
	if lt.name != "" {
		L.Push(LString(lt.name))
	} else {
		L.Push(LNil)
	}
	return 1
}

func typeMethodElem(L *LState, lt *LType) int {
	if arr, ok := lt.inner.(*typ.Array); ok && arr.Element != nil {
		L.Push(&LType{inner: arr.Element})
		return 1
	}
	L.Push(LNil)
	return 1
}

func typeMethodKey(L *LState, lt *LType) int {
	if m, ok := lt.inner.(*typ.Map); ok && m.Key != nil {
		L.Push(&LType{inner: m.Key})
		return 1
	}
	L.Push(LNil)
	return 1
}

func typeMethodVal(L *LState, lt *LType) int {
	if m, ok := lt.inner.(*typ.Map); ok && m.Value != nil {
		L.Push(&LType{inner: m.Value})
		return 1
	}
	L.Push(LNil)
	return 1
}

func typeMethodInner(L *LState, lt *LType) int {
	if opt, ok := lt.inner.(*typ.Optional); ok && opt.Inner != nil {
		L.Push(&LType{inner: opt.Inner})
		return 1
	}
	L.Push(LNil)
	return 1
}

func typeMethodRet(L *LState, lt *LType) int {
	if fn, ok := lt.inner.(*typ.Function); ok {
		if len(fn.Returns) == 1 && fn.Returns[0] != nil {
			L.Push(&LType{inner: fn.Returns[0]})
			return 1
		}
		if len(fn.Returns) > 1 {
			L.Push(&LType{inner: typ.NewTuple(fn.Returns...)})
			return 1
		}
	}
	L.Push(LNil)
	return 1
}

func typeMethodFields(L *LState, lt *LType) int {
	rec, ok := lt.inner.(*typ.Record)
	if !ok {
		L.Push(LNil)
		return 1
	}

	idx := 0
	iter := LGoFunc(func(L *LState) int {
		if idx >= len(rec.Fields) {
			L.Push(LNil)
			return 1
		}
		field := rec.Fields[idx]
		idx++
		L.Push(LString(field.Name))
		if field.Type != nil {
			L.Push(&LType{inner: field.Type})
		} else {
			L.Push(LNil)
		}
		return 2
	})

	L.Push(iter)
	return 1
}

func typeMethodVariants(L *LState, lt *LType) int {
	union, ok := lt.inner.(*typ.Union)
	if !ok {
		L.Push(LNil)
		return 1
	}

	idx := 0
	iter := LGoFunc(func(L *LState) int {
		if idx >= len(union.Members) {
			L.Push(LNil)
			return 1
		}
		variant := union.Members[idx]
		idx++
		if variant != nil {
			L.Push(&LType{inner: variant})
		} else {
			L.Push(LNil)
		}
		return 1
	})

	L.Push(iter)
	return 1
}

func typeMethodParams(L *LState, lt *LType) int {
	fn, ok := lt.inner.(*typ.Function)
	if !ok {
		L.Push(LNil)
		return 1
	}

	idx := 0
	iter := LGoFunc(func(L *LState) int {
		if idx >= len(fn.Params) {
			L.Push(LNil)
			return 1
		}
		param := fn.Params[idx]
		idx++
		if param.Type != nil {
			L.Push(&LType{inner: param.Type})
		} else {
			L.Push(LNil)
		}
		return 1
	})

	L.Push(iter)
	return 1
}

func typeMethodTparams(L *LState, lt *LType) int {
	gen, ok := lt.inner.(*typ.Generic)
	if !ok {
		L.Push(LNil)
		return 1
	}

	idx := 0
	iter := LGoFunc(func(L *LState) int {
		if idx >= len(gen.TypeParams) {
			L.Push(LNil)
			return 1
		}
		param := gen.TypeParams[idx]
		idx++
		L.Push(LString(param.Name))
		if param.Constraint != nil {
			L.Push(&LType{inner: param.Constraint})
		} else {
			L.Push(LNil)
		}
		return 2
	})

	L.Push(iter)
	return 1
}

// typeCall handles calling a type: Type(value) for validation,
// or Type(type_args...) for generic instantiation.
func (ls *LState) typeCall(lt *LType, base, nargs, nret int) {
	if nargs == 0 {
		ls.RaiseError("type %s expects at least 1 argument", lt.String())
		return
	}

	// Count type arguments to distinguish generic instantiation vs validation.
	typeArgCount := 0
	for i := 1; i <= nargs; i++ {
		if _, ok := ls.reg.Get(base + i).(*LType); !ok {
			continue
		}
		typeArgCount++
	}

	switch typeArgCount {
	case nargs:
		// Generic instantiation
		gen, ok := lt.inner.(*typ.Generic)
		if !ok {
			ls.RaiseError("type %s is not generic", lt.String())
			return
		}

		if nargs != len(gen.TypeParams) {
			ls.RaiseError("generic %s expects %d type arguments, got %d",
				lt.String(), len(gen.TypeParams), nargs)
			return
		}

		args := make([]typ.Type, nargs)
		for i := 0; i < nargs; i++ {
			args[i] = ls.reg.Get(base + 1 + i).(*LType).inner
		}

		instantiated := typ.Instantiate(gen, args...)

		result := &LType{inner: instantiated, name: lt.name}
		if nret != 0 {
			ls.reg.Set(base, result)
			if nret > 1 {
				for i := 1; i < nret; i++ {
					ls.reg.Set(base+i, LNil)
				}
			}
		}
		if nret < 0 {
			ls.reg.SetTop(base + 1)
		}
	case 0:
		if nargs != 1 {
			ls.RaiseError("type %s validation expects 1 value, got %d", lt.String(), nargs)
			return
		}
		// Validation
		val := ls.reg.Get(base + 1)
		if !lt.Validate(ls, val) {
			ls.RaiseError("expected %s, got %s", lt.String(), val.Type().String())
			return
		}

		if nret != 0 {
			ls.reg.Set(base, val)
			if nret > 1 {
				for i := 1; i < nret; i++ {
					ls.reg.Set(base+i, LNil)
				}
			}
		}
		if nret < 0 {
			ls.reg.SetTop(base + 1)
		}
	default:
		ls.RaiseError("type %s does not accept mixed type/value arguments", lt.String())
		return
	}
}

// TypeEquals checks structural equality of two types.
func TypeEquals(a, b *LType) bool {
	if a.inner == nil || b.inner == nil {
		return a.inner == nil && b.inner == nil
	}
	return a.inner.Equals(b.inner)
}

// TypeIsSubtype checks if a is a subtype of b.
func TypeIsSubtype(a, b *LType) bool {
	if a.inner == nil || b.inner == nil {
		return false
	}
	return subtype.IsSubtype(a.inner, b.inner)
}

// Debug helper
func (lt *LType) GoString() string {
	return fmt.Sprintf("LType{%s}", lt.String())
}
