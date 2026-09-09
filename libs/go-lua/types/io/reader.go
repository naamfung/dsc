package io

import (
	"bytes"
	"io"
	"math"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

const maxTypeDepth = 256
const maxTypeNodes = 1 << 20
const maxCollectionLen = 1 << 20
const maxStringLen = 16 << 20

type typeReader struct {
	r   *bytes.Reader
	err error

	recursive map[uint64]*typ.Recursive
	depth     int
	nodeCount int
}

func (r *typeReader) readByte() byte {
	if r.err != nil {
		return 0
	}

	b, err := r.r.ReadByte()
	if err != nil {
		r.err = err
		return 0
	}

	return b
}

func (r *typeReader) readUint32() uint32 {
	if r.err != nil {
		return 0
	}

	var v uint32
	r.err = binaryRead(r.r, &v)

	return v
}

func (r *typeReader) readUint64() uint64 {
	if r.err != nil {
		return 0
	}

	var v uint64
	r.err = binaryRead64(r.r, &v)

	return v
}

func (r *typeReader) readString() string {
	if r.err != nil {
		return ""
	}

	length := r.readUint32()
	if length == 0 {
		return ""
	}

	if length > maxStringLen || uint64(length) > uint64(r.r.Len()) {
		r.err = ErrCorruptedData
		return ""
	}

	data := make([]byte, length)
	_, r.err = io.ReadFull(r.r, data)

	return string(data)
}

func (r *typeReader) readBool() bool {
	return r.readByte() == 1
}

func (r *typeReader) checkSliceLen(n uint32) bool {
	if n > maxCollectionLen || uint64(n) > uint64(r.r.Len()) {
		r.err = ErrCorruptedData
		return false
	}

	return true
}

func (r *typeReader) readType() typ.Type {
	if r.err != nil {
		return nil
	}
	r.depth++
	r.nodeCount++
	if r.depth > maxTypeDepth || r.nodeCount > maxTypeNodes {
		r.err = ErrCorruptedData
		return nil
	}
	defer func() { r.depth-- }()

	k := byteToKind(r.readByte())

	switch k {
	case kind.Nil:
		return typ.Nil
	case kind.Boolean:
		return typ.Boolean
	case kind.Number:
		return typ.Number
	case kind.Integer:
		return typ.Integer
	case kind.String:
		return typ.String
	case kind.Any:
		return typ.Any
	case kind.Unknown:
		return typ.Unknown
	case kind.Never:
		return typ.Never
	case kind.Self:
		return typ.Self

	case kind.Optional:
		inner := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
		return typ.NewOptional(inner)

	case kind.Union:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		members := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			members[i] = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		return typ.NewUnion(members...)

	case kind.Intersection:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		members := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			members[i] = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		return typ.NewIntersection(members...)

	case kind.Tuple:
		count := r.readUint32()
		if !r.checkSliceLen(count) {
			return nil
		}

		elems := make([]typ.Type, count)
		for i := uint32(0); i < count; i++ {
			elems[i] = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		return typ.NewTuple(elems...)

	case kind.Function:
		typeParamCount := r.readUint32()
		if !r.checkSliceLen(typeParamCount) {
			return nil
		}

		fb := typ.Func()
		for i := uint32(0); i < typeParamCount; i++ {
			tp := r.readTypeParam()
			if r.err != nil || tp == nil {
				return nil
			}
			fb.TypeParam(tp.Name, tp.Constraint)
		}

		paramCount := r.readUint32()
		if !r.checkSliceLen(paramCount) {
			return nil
		}

		for i := uint32(0); i < paramCount; i++ {
			name := r.readString()
			pType := r.readTypeNonNil()

			if r.err != nil {
				return nil
			}

			optional := r.readBool()
			if optional {
				fb.OptParam(name, pType)
			} else {
				fb.Param(name, pType)
			}
		}

		if r.readBool() {
			variadic := r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
			fb.Variadic(variadic)
		}

		retCount := r.readUint32()
		if !r.checkSliceLen(retCount) {
			return nil
		}

		returns := make([]typ.Type, retCount)
		for i := uint32(0); i < retCount; i++ {
			returns[i] = r.readTypeNonNil()

			if r.err != nil {
				return nil
			}
		}

		fb.Returns(returns...)

		eff := r.readEffectRow()
		fb.Effects(eff)

		if r.readBool() {
			spec := r.readContractSpec()
			if spec != nil {
				fb.Spec(spec)
			}
		}

		if refinement := r.readFunctionRefinement(); refinement != nil {
			fb.WithRefinement(refinement)
		}

		return fb.Build()

	case kind.Array:
		elem := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
		return typ.NewArray(elem)

	case kind.Map:
		key := r.readTypeNonNil()
		value := r.readTypeNonNil()

		if r.err != nil {
			return nil
		}

		return typ.NewMap(key, value)

	case kind.Record:
		fieldCount := r.readUint32()
		if !r.checkSliceLen(fieldCount) {
			return nil
		}

		rb := typ.NewRecord()

		for i := uint32(0); i < fieldCount; i++ {
			name := r.readString()
			fType := r.readTypeNonNil()

			if r.err != nil {
				return nil
			}

			optional := r.readBool()
			readonly := r.readBool()

			switch {
			case readonly:
				rb.ReadonlyField(name, fType)
			case optional:
				rb.OptField(name, fType)
			default:
				rb.Field(name, fType)
			}
		}

		if r.readBool() {
			rb.Metatable(r.readTypeNonNil())
			if r.err != nil {
				return nil
			}
		}
		open := r.readBool()
		rb.SetOpen(open)
		hasMap := r.readBool()
		if hasMap {
			key := r.readTypeNonNil()
			value := r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
			rb.MapComponent(key, value)
		}

		return rb.Build()

	case kind.Generic:
		name := r.readString()

		paramCount := r.readUint32()
		if !r.checkSliceLen(paramCount) {
			return nil
		}

		params := make([]*typ.TypeParam, paramCount)
		for i := uint32(0); i < paramCount; i++ {
			params[i] = r.readTypeParam()
			if r.err != nil || params[i] == nil {
				return nil
			}
		}

		var body typ.Type
		if r.readBool() {
			body = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		return typ.NewGeneric(name, params, body)

	case kind.Instantiated:
		var generic *typ.Generic

		if r.readBool() {
			g := r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
			gen, ok := g.(*typ.Generic)
			if !ok {
				r.err = ErrInvalidType
				return nil
			}
			generic = gen
		}

		argCount := r.readUint32()
		if !r.checkSliceLen(argCount) {
			return nil
		}

		args := make([]typ.Type, argCount)
		for i := uint32(0); i < argCount; i++ {
			args[i] = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		if generic == nil {
			return nil
		}
		return typ.Instantiate(generic, args...)

	case kind.TypeParam:
		name := r.readString()

		var constr typ.Type
		if r.readBool() {
			constr = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}

		return typ.NewTypeParam(name, constr)

	case kind.TypeVar:
		return typ.NewTypeVar(int(r.readUint32()))

	case kind.Literal:
		return r.readLiteral()

	case kind.Ref:
		module := r.readString()
		name := r.readString()

		return typ.NewRef(module, name)

	case kind.Alias:
		name := r.readString()
		target := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}

		return typ.NewAlias(name, target)

	case kind.Meta:
		target := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
		return typ.NewMeta(target)

	case kind.Platform:
		return typ.NewPlatform(r.readString())

	case kind.Refined:
		inner := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
		annCount := r.readUint32()
		if !r.checkSliceLen(annCount) {
			return nil
		}
		annotations := make([]typ.Annotation, annCount)
		for i := uint32(0); i < annCount; i++ {
			name := r.readString()
			arg := r.readAnnotationArg()
			if r.err != nil {
				return nil
			}
			annotations[i] = typ.Annotation{Name: name, Arg: arg}
		}
		return typ.NewAnnotated(inner, annotations)

	case kind.FieldAccess:
		base := r.readTypeNonNil()
		field := r.readString()
		if r.err != nil {
			return nil
		}
		return typ.NewFieldAccess(base, field)

	case kind.IndexAccess:
		base := r.readTypeNonNil()
		index := r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
		return typ.NewIndexAccess(base, index)

	case kind.Sum:
		name := r.readString()
		variantCount := r.readUint32()
		if !r.checkSliceLen(variantCount) {
			return nil
		}
		variants := make([]typ.Variant, variantCount)
		for i := uint32(0); i < variantCount; i++ {
			tag := r.readString()
			typeCount := r.readUint32()
			if !r.checkSliceLen(typeCount) {
				return nil
			}
			types := make([]typ.Type, typeCount)
			for j := uint32(0); j < typeCount; j++ {
				types[j] = r.readTypeNonNil()
				if r.err != nil {
					return nil
				}
			}
			variants[i] = typ.Variant{Tag: tag, Types: types}
		}
		return typ.NewSum(name, variants)

	case kind.Interface:
		name := r.readString()
		methodCount := r.readUint32()
		if !r.checkSliceLen(methodCount) {
			return nil
		}
		methods := make([]typ.Method, methodCount)
		for i := uint32(0); i < methodCount; i++ {
			methodName := r.readString()
			methodType := r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
			fn, ok := methodType.(*typ.Function)
			if !ok {
				r.err = ErrInvalidType
				return nil
			}
			methods[i] = typ.Method{Name: methodName, Type: fn}
		}
		return typ.NewInterface(name, methods)

	case kind.Recursive:
		encodedID := r.readUint64()
		name := r.readString()
		hasBody := r.readBool()
		rec := r.getOrCreateRecursive(encodedID, name)
		if r.err != nil {
			return nil
		}
		if hasBody {
			if rec.Body != nil {
				r.err = ErrInvalidType
				return nil
			}
			body := r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
			rec.Body = body
		}
		return rec
	}

	r.err = ErrUnknownType
	return nil
}

func (r *typeReader) readTypeNonNil() typ.Type {
	t := r.readType()
	if r.err != nil {
		return nil
	}
	if t == nil {
		r.err = ErrInvalidType
	}
	return t
}

func (r *typeReader) getOrCreateRecursive(encodedID uint64, name string) *typ.Recursive {
	if r.recursive == nil {
		r.recursive = make(map[uint64]*typ.Recursive)
	}
	if rec, ok := r.recursive[encodedID]; ok {
		if rec.Name == "" && name != "" {
			rec.Name = name
		}
		return rec
	}
	rec := typ.NewRecursivePlaceholder(name)
	r.recursive[encodedID] = rec
	return rec
}

func (r *typeReader) readAnnotationArg() any {
	tag := r.readByte()
	switch tag {
	case annotationArgNil:
		return nil
	case annotationArgString:
		return r.readString()
	case annotationArgInt:
		return int64(r.readUint64())
	case annotationArgFloat:
		return math.Float64frombits(r.readUint64())
	case annotationArgBool:
		return r.readBool()
	default:
		r.err = ErrInvalidType
		return nil
	}
}

func (r *typeReader) readTypeParam() *typ.TypeParam {
	name := r.readString()

	var constr typ.Type
	if r.readBool() {
		constr = r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
	}

	return typ.NewTypeParam(name, constr)
}

func (r *typeReader) readLiteral() *typ.Literal {
	base := byteToKind(r.readByte())
	switch base {
	case kind.Boolean:
		return typ.LiteralBool(r.readBool())
	case kind.Integer:
		return typ.LiteralInt(int64(r.readUint64()))
	case kind.Number:
		return typ.LiteralNumber(math.Float64frombits(r.readUint64()))
	case kind.String:
		return typ.LiteralString(r.readString())
	case kind.Nil, kind.Any, kind.Unknown, kind.Never, kind.Optional, kind.Union,
		kind.Intersection, kind.Tuple, kind.Function, kind.Array, kind.Map, kind.Record,
		kind.Sum, kind.Interface, kind.Alias, kind.Generic, kind.Instantiated, kind.Platform,
		kind.Literal, kind.Self, kind.Ref, kind.Meta, kind.TypeParam, kind.TypeVar,
		kind.Refined, kind.FieldAccess, kind.IndexAccess, kind.Recursive:
		r.err = ErrInvalidType
		return nil
	}

	r.err = ErrInvalidType
	return nil
}

func (r *typeReader) readPath() constraint.Path {
	root := r.readString()
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return constraint.Path{}
	}

	segs := make([]constraint.Segment, 0, count)

	for i := uint32(0); i < count; i++ {
		segKind := constraint.SegmentKind(r.readByte())
		seg := constraint.Segment{Kind: segKind}

		switch segKind {
		case constraint.SegmentField, constraint.SegmentIndexString:
			seg.Name = r.readString()
		case constraint.SegmentIndexInt:
			seg.Index = int(int32(r.readUint32()))
		}

		segs = append(segs, seg)
	}

	return constraint.Path{Root: root, Segments: segs}
}

func (r *typeReader) readTypeKey() narrow.TypeKey {
	k := narrow.TypeKeyKind(r.readByte())
	switch k {
	case narrow.TypeKeyBuiltin:
		return narrow.TypeKey{Kind: k, Name: r.readString()}
	case narrow.TypeKeyHash:
		return narrow.TypeKey{Kind: k, Hash: r.readUint64()}
	case narrow.TypeKeyInvalid:
		return narrow.TypeKey{}
	}

	return narrow.TypeKey{}
}

func (r *typeReader) readCallbackSpec() *contract.CallbackSpec {
	if !r.readBool() {
		return nil
	}

	index := int(int32(r.readUint32()))

	cb := &contract.CallbackSpec{
		InputSource:    effect.ParamRef{Index: index},
		ReturnsBoolean: r.readBool(),
		Cardinality:    contract.Cardinality(r.readUint32()),
		Pure:           r.readBool(),
	}

	envCount := r.readUint32()
	if envCount > 0 {
		if !r.checkSliceLen(envCount) {
			return cb
		}

		cb.EnvOverlay = make(map[string]typ.Type, envCount)

		for i := uint32(0); i < envCount; i++ {
			name := r.readString()
			cb.EnvOverlay[name] = r.readTypeNonNil()
			if r.err != nil {
				return nil
			}
		}
	}

	return cb
}

func (r *typeReader) readContractSpec() *contract.Spec {
	spec := contract.NewSpec()
	spec.Requires = r.readCondition()
	spec.Ensures = r.readCondition()
	spec.ExprRequires = r.readExprCompares()
	spec.ExprEnsures = r.readExprCompares()
	spec.Effects = r.readEffectRow()
	cbCount := r.readUint32()

	if !r.checkSliceLen(cbCount) {
		return spec
	}

	for i := uint32(0); i < cbCount; i++ {
		key := int(r.readUint32())

		if spec.Callbacks == nil {
			spec.Callbacks = make(map[int]*contract.CallbackSpec)
		}

		if cb := r.readCallbackSpec(); cb != nil {
			spec.Callbacks[key] = cb
		}
	}

	spec.Return = r.readReturnSpec()

	return spec
}

func (r *typeReader) readReturnSpec() *contract.ReturnSpec {
	if !r.readBool() {
		return nil
	}

	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return nil
	}

	rs := &contract.ReturnSpec{
		Cases: make([]contract.ReturnCase, count),
	}
	for i := uint32(0); i < count; i++ {
		rs.Cases[i] = contract.ReturnCase{
			When: r.readCondition(),
			Type: r.readTypeNonNil(),
		}
		if r.err != nil {
			return nil
		}
	}

	if r.readBool() {
		rs.Default = r.readTypeNonNil()
		if r.err != nil {
			return nil
		}
	}

	return rs
}

func (r *typeReader) readEffectRow() effect.Row {
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return effect.Empty
	}

	labels := make([]effect.Label, 0, count)

	for i := uint32(0); i < count; i++ {
		if label := r.readEffectLabel(); label != nil {
			labels = append(labels, label)
		}
	}

	var tail *effect.Var
	if r.readBool() {
		tail = &effect.Var{Name: r.readString()}
	}

	return effect.Row{Labels: labels, Tail: tail}
}

func (r *typeReader) readEffectLabel() effect.Label {
	key := r.readString()
	if key == "" {
		return nil
	}

	codec, ok := effect.Lookup(key)
	if !ok {
		return nil
	}

	label, _ := codec.Decode(&readerAdapter{r})

	return label
}

type readerAdapter struct{ r *typeReader }

func (a *readerAdapter) ReadByte() (byte, error) {
	return a.r.readByte(), a.r.err
}

func (a *readerAdapter) ReadInt32() (int32, error) {
	return int32(a.r.readUint32()), a.r.err
}

func (a *readerAdapter) ReadString() (string, error) {
	return a.r.readString(), a.r.err
}

func (a *readerAdapter) ReadType() (any, error) {
	return a.r.readType(), a.r.err
}
