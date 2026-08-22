package typ

import (
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/kind"
)

// Field represents a record field with name, type, optionality, and mutability.
type Field struct {
	Name     string
	Type     Type
	Optional bool // True if field may be absent (nil access returns nil)
	Readonly bool // True if field cannot be reassigned
}

// Record represents a Lua table with named fields: {field1: T1, field2: T2, ...}.
//
// Records support both structural typing (field presence/type matching) and
// optional map components for tables with dynamic indexing.
//
// Features:
//   - Open: When true, unknown field access returns Unknown instead of error
//   - MapKey/MapValue: Optional map component for {foo: T, [K]: V} patterns
//   - Metatable: Optional metatable type for metamethod resolution
//
// Fields are sorted by name for deterministic hashing and comparison.
type Record struct {
	Fields       []Field
	Metatable    Type // Metatable type for metamethod lookup
	MapKey       Type // Map component key type (nil if no map component)
	MapValue     Type // Map component value type (nil if no map component)
	Open         bool // Allow access to undefined fields
	sorted       bool
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// RecordBuilder provides a fluent API for constructing record types.
//
// Example:
//
//	rec := typ.NewRecord().
//	    Field("name", typ.String).
//	    OptField("age", typ.Integer).
//	    Build()
type RecordBuilder struct {
	fields    []Field
	metatable Type
	mapKey    Type
	mapValue  Type
	open      bool
}

// NewRecord starts building a record type.
func NewRecord() *RecordBuilder {
	return &RecordBuilder{}
}

// Field adds a required field.
func (b *RecordBuilder) Field(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t})
	return b
}

// OptField adds an optional field.
func (b *RecordBuilder) OptField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Optional: true})
	return b
}

// ReadonlyField adds a readonly field.
func (b *RecordBuilder) ReadonlyField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Readonly: true})
	return b
}

// OptReadonlyField adds an optional readonly field.
func (b *RecordBuilder) OptReadonlyField(name string, t Type) *RecordBuilder {
	b.fields = append(b.fields, Field{Name: name, Type: t, Optional: true, Readonly: true})
	return b
}

// AnnotatedField adds a field with validation annotations.
func (b *RecordBuilder) AnnotatedField(name string, t Type, optional bool, annotations []Annotation) *RecordBuilder {
	if len(annotations) > 0 {
		t = NewAnnotated(t, annotations)
	}
	if optional {
		return b.OptField(name, t)
	}
	return b.Field(name, t)
}

// Metatable sets the metatable type.
func (b *RecordBuilder) Metatable(t Type) *RecordBuilder {
	b.metatable = t
	return b
}

// SetOpen marks the record as open (unknown field access returns unknown).
func (b *RecordBuilder) SetOpen(open bool) *RecordBuilder {
	b.open = open
	return b
}

// MapComponent sets the map component key and value types.
func (b *RecordBuilder) MapComponent(key, value Type) *RecordBuilder {
	b.mapKey = key
	b.mapValue = value
	return b
}

// Build creates the record type.
func (b *RecordBuilder) Build() *Record {
	return buildRecordType(b.fields, b.metatable, b.mapKey, b.mapValue, b.open, false)
}

func (r *Record) Kind() kind.Kind { return kind.Record }

func (r *Record) String() string {
	return r.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString("{")

		for i, f := range r.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}

			if f.Readonly {
				sb.WriteString("readonly ")
			}

			sb.WriteString(f.Name)

			if f.Optional {
				sb.WriteString("?")
			}

			sb.WriteString(": ")
			if f.Type != nil {
				sb.WriteString(f.Type.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.HasMapComponent() {
			if len(r.Fields) > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("[")
			if r.MapKey != nil {
				sb.WriteString(r.MapKey.String())
			} else {
				sb.WriteString("unknown")
			}
			sb.WriteString("]: ")
			if r.MapValue != nil {
				sb.WriteString(r.MapValue.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.Open {
			if len(r.Fields) > 0 || r.HasMapComponent() {
				sb.WriteString(", ")
			}
			sb.WriteString("...")
		}

		sb.WriteString("}")

		return sb.String()
	})
}

func (r *Record) Hash() uint64 { return r.hash }

func (r *Record) Equals(other Type) bool {
	return TypeEquals(r, other)
}

// HasMapComponent returns true if the record has a map component (MapKey and MapValue set).
func (r *Record) HasMapComponent() bool {
	return r.MapKey != nil && r.MapValue != nil
}

// GetField returns the field with the given name, or nil.
func (r *Record) GetField(name string) *Field {
	if r.sorted {
		i := sort.Search(len(r.Fields), func(i int) bool {
			return r.Fields[i].Name >= name
		})
		if i < len(r.Fields) && r.Fields[i].Name == name {
			return &r.Fields[i]
		}
		return nil
	}

	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return &r.Fields[i]
		}
	}

	return nil
}
