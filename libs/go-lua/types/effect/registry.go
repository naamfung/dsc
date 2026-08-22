package effect

import "sync"

// Codec key constants define the serialization identifiers for each label type.
//
// These keys are written to the binary format before each label's payload,
// enabling the decoder to select the appropriate codec for deserialization.
// Keys should be stable across versions to maintain backward compatibility.
const (
	KeyThrow             = "throw"
	KeyIO                = "io"
	KeyDiverge           = "diverge"
	KeyMutate            = "mutate"
	KeyReturn            = "return"
	KeyErrorReturn       = "error_return"
	KeyReturnLength      = "return_length"
	KeyIterator          = "iterator"
	KeyTableMutator      = "table_mutator"
	KeyLengthChange      = "length_change"
	KeyBorrow            = "borrow"
	KeyStore             = "store"
	KeyBorrowAll         = "borrow_all"
	KeyPassThrough       = "passthrough"
	KeyFlowInto          = "flowinto"
	KeySend              = "send"
	KeyFreeze            = "freeze"
	KeyCorrelatedReturn  = "correlated_return"
	KeyModuleLoad        = "module_load"
	KeyVariadicTransform = "variadic_transform"
	KeyTypePredicate     = "type_predicate"
	KeyTypeValueMethod   = "type_value_method"
	KeyCallableType      = "callable_type"
)

// LabelCodec handles binary serialization for a label type.
//
// Each Label implementation has a corresponding codec registered in the global
// registry. The codec is responsible for encoding and decoding the label's
// data to/from the binary format.
//
// Implementations:
//   - Key(): Returns the codec key constant for this label type
//   - Encode(): Writes the label's fields to the Writer
//   - Decode(): Reads fields from the Reader and constructs a Label
type LabelCodec interface {
	Key() string
	Encode(l Label, w Writer) error
	Decode(r Reader) (Label, error)
}

// Writer provides methods for serializing effect data to binary format.
//
// Implementations write to an underlying byte stream in little-endian format.
// The types/io package provides the concrete implementation used for manifest
// serialization.
type Writer interface {
	WriteByte(b byte) error
	WriteInt32(v int32) error
	WriteString(s string) error
	WriteType(t any) error
}

// Reader provides methods for deserializing effect data from binary format.
//
// Implementations read from an underlying byte stream in little-endian format.
// The types/io package provides the concrete implementation used for manifest
// deserialization.
type Reader interface {
	ReadByte() (byte, error)
	ReadInt32() (int32, error)
	ReadString() (string, error)
	ReadType() (any, error)
}

var (
	registryMu sync.RWMutex
	codecs     = make(map[string]LabelCodec)
)

// Register adds a label codec to the global registry.
//
// Call from init() to ensure the codec is available before any serialization
// occurs. The codecs.go file registers all built-in label codecs.
func Register(c LabelCodec) {
	registryMu.Lock()
	defer registryMu.Unlock()

	codecs[c.Key()] = c
}

// Lookup returns the codec for the given key, if registered.
//
// Used during deserialization to find the appropriate decoder for each label
// in the binary stream.
func Lookup(key string) (LabelCodec, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	c, ok := codecs[key]

	return c, ok
}

// LabelKey returns the canonical codec key for a label instance.
//
// Maps each Label implementation to its corresponding Key constant for
// serialization. Falls back to the label's String() for unknown types.
func LabelKey(l Label) string {
	return VisitLabel(l, LabelVisitor[string]{
		Throw: func(Throw) string {
			return KeyThrow
		},
		IO: func(IO) string {
			return KeyIO
		},
		Diverge: func(Diverge) string {
			return KeyDiverge
		},
		Mutate: func(Mutate) string {
			return KeyMutate
		},
		Return: func(Return) string {
			return KeyReturn
		},
		ErrorReturn: func(ErrorReturn) string {
			return KeyErrorReturn
		},
		ReturnLength: func(ReturnLength) string {
			return KeyReturnLength
		},
		Iterator: func(Iterator) string {
			return KeyIterator
		},
		TableMutator: func(TableMutator) string {
			return KeyTableMutator
		},
		LengthChange: func(LengthChange) string {
			return KeyLengthChange
		},
		Borrow: func(Borrow) string {
			return KeyBorrow
		},
		Store: func(Store) string {
			return KeyStore
		},
		BorrowAll: func(BorrowAll) string {
			return KeyBorrowAll
		},
		PassThrough: func(PassThrough) string {
			return KeyPassThrough
		},
		FlowInto: func(FlowInto) string {
			return KeyFlowInto
		},
		Send: func(Send) string {
			return KeySend
		},
		Freeze: func(Freeze) string {
			return KeyFreeze
		},
		CorrelatedReturn: func(CorrelatedReturn) string {
			return KeyCorrelatedReturn
		},
		ModuleLoad: func(ModuleLoad) string {
			return KeyModuleLoad
		},
		VariadicTransform: func(VariadicTransform) string {
			return KeyVariadicTransform
		},
		TypePredicate: func(TypePredicate) string {
			return KeyTypePredicate
		},
		TypeValueMethod: func(TypeValueMethod) string {
			return KeyTypeValueMethod
		},
		CallableType: func(CallableType) string {
			return KeyCallableType
		},
		Default: func(l Label) string {
			return l.String()
		},
	})
}

// CodecFor returns the codec for a label instance.
//
// Combines LabelKey and Lookup for convenience when serializing a label.
func CodecFor(l Label) (LabelCodec, bool) {
	return Lookup(LabelKey(l))
}
