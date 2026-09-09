package typ

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/kind"
)

// Annotation represents a runtime validation constraint.
type Annotation struct {
	Name string
	Arg  any
}

// Annotated wraps a type with runtime validation annotations.
// The underlying type determines structural typing while annotations
// add runtime constraints like @min(0), @max(100), @pattern("^.+$").
type Annotated struct {
	Inner       Type
	Annotations []Annotation
	hash        uint64
	strCache    stringCache
}

// NewAnnotated creates an annotated type wrapper.
// Returns the inner type directly if no annotations are provided.
func NewAnnotated(inner Type, annotations []Annotation) Type {
	if len(annotations) == 0 {
		return inner
	}
	if inner == nil {
		inner = Unknown
	}
	h := internal.HashCombine(inner.Hash(), uint64(kind.Platform)<<32)
	for _, ann := range annotations {
		h = internal.HashCombine(h, internal.FnvString(ann.Name))
	}
	return &Annotated{
		Inner:       inner,
		Annotations: annotations,
		hash:        h,
	}
}

func (a *Annotated) Kind() kind.Kind {
	if a.Inner == nil {
		return kind.Unknown
	}
	return a.Inner.Kind()
}

func (a *Annotated) String() string {
	return a.strCache.get(func() string {
		var sb strings.Builder
		if a.Inner != nil {
			sb.WriteString(a.Inner.String())
		} else {
			sb.WriteString("unknown")
		}
		for _, ann := range a.Annotations {
			sb.WriteString(" @")
			sb.WriteString(ann.Name)
			if ann.Arg != nil {
				sb.WriteString("(")
				switch v := ann.Arg.(type) {
				case string:
					sb.WriteString("\"")
					sb.WriteString(v)
					sb.WriteString("\"")
				case float64:
					sb.WriteString(formatFloat(v))
				case int64:
					sb.WriteString(formatInt(v))
				case int:
					sb.WriteString(formatInt(int64(v)))
				default:
					sb.WriteString("...")
				}
				sb.WriteString(")")
			}
		}
		return sb.String()
	})
}

func (a *Annotated) Hash() uint64 {
	return a.hash
}

func (a *Annotated) Equals(other Type) bool {
	o, ok := other.(*Annotated)
	if !ok {
		return false
	}
	if !a.Inner.Equals(o.Inner) {
		return false
	}
	if len(a.Annotations) != len(o.Annotations) {
		return false
	}
	for i, ann := range a.Annotations {
		if ann.Name != o.Annotations[i].Name {
			return false
		}
	}
	return true
}

// UnwrapAnnotated returns the inner type, stripping annotations.
func UnwrapAnnotated(t Type) Type {
	if a, ok := t.(*Annotated); ok {
		if a.Inner == nil {
			return Unknown
		}
		return a.Inner
	}
	return t
}

// GetAnnotations returns annotations from a type, or nil if none.
func GetAnnotations(t Type) []Annotation {
	if a, ok := t.(*Annotated); ok {
		return a.Annotations
	}
	return nil
}

func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return formatInt(int64(f))
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatInt(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte(i%10) + '0'
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
