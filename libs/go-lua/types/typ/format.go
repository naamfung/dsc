package typ

import (
	"strings"

	"github.com/wippyai/go-lua/internal"
)

// FormatOptions controls budgeted type rendering for diagnostics.
// Limits are best-effort; rendering may truncate with "..." when exceeded.
type FormatOptions struct {
	MaxDepth        int
	MaxNodes        int
	MaxUnionMembers int
	MaxRecordFields int
	MaxTupleElems   int
	MaxTypeParams   int
	MaxParams       int
	MaxReturns      int
	MaxBytes        int
}

// DefaultFormatOptions keeps diagnostics readable while avoiding huge output.
var DefaultFormatOptions = FormatOptions{
	MaxDepth:        6,
	MaxNodes:        200,
	MaxUnionMembers: 6,
	MaxRecordFields: 8,
	MaxTupleElems:   8,
	MaxTypeParams:   6,
	MaxParams:       8,
	MaxReturns:      6,
	MaxBytes:        800,
}

// FormatShort renders a type for diagnostics with bounded output size.
func FormatShort(t Type) string {
	return Format(t, DefaultFormatOptions)
}

// Format renders a type using the provided options.
func Format(t Type, opts FormatOptions) string {
	f := formatter{
		opts: opts,
	}
	f.formatType(t, 0, NewGuard())
	return f.string()
}

type formatter struct {
	opts      FormatOptions
	nodes     int
	bytes     int
	truncated bool
	sb        strings.Builder
}

func (f *formatter) string() string {
	s := f.sb.String()
	if f.truncated && !strings.HasSuffix(s, "...") {
		if f.bytes+3 <= f.opts.MaxBytes {
			f.sb.WriteString("...")
			s = f.sb.String()
		}
	}
	return s
}

func (f *formatter) write(s string) {
	if f.truncated {
		return
	}
	if f.opts.MaxBytes > 0 && f.bytes >= f.opts.MaxBytes {
		f.truncated = true
		return
	}
	if f.opts.MaxBytes > 0 {
		remaining := f.opts.MaxBytes - f.bytes
		if remaining <= 0 {
			f.truncated = true
			return
		}
		if len(s) > remaining {
			f.sb.WriteString(s[:remaining])
			f.bytes += remaining
			f.truncated = true
			return
		}
	}
	f.sb.WriteString(s)
	f.bytes += len(s)
}

func (f *formatter) formatType(t Type, depth int, guard internal.RecursionGuard) {
	if f.truncated {
		return
	}
	if t == nil {
		f.write("nil")
		return
	}
	if f.opts.MaxDepth > 0 && depth > f.opts.MaxDepth {
		f.write("...")
		return
	}
	f.nodes++
	if f.opts.MaxNodes > 0 && f.nodes > f.opts.MaxNodes {
		f.write("...")
		f.truncated = true
		return
	}

	VisitWithGuard(t, guard, struct{}{}, func(next internal.RecursionGuard) Visitor[struct{}] {
		return Visitor[struct{}]{
			Union: func(u *Union) struct{} {
				f.formatUnion(u, depth, next)
				return struct{}{}
			},
			Intersection: func(u *Intersection) struct{} {
				f.formatIntersection(u, depth, next)
				return struct{}{}
			},
			Optional: func(o *Optional) struct{} {
				f.formatType(o.Inner, depth+1, next)
				f.write("?")
				return struct{}{}
			},
			Array: func(a *Array) struct{} {
				f.formatType(a.Element, depth+1, next)
				f.write("[]")
				return struct{}{}
			},
			Map: func(m *Map) struct{} {
				f.write("{[")
				f.formatType(m.Key, depth+1, next)
				f.write("]: ")
				f.formatType(m.Value, depth+1, next)
				f.write("}")
				return struct{}{}
			},
			Tuple: func(tu *Tuple) struct{} {
				f.formatTuple(tu, depth, next)
				return struct{}{}
			},
			Function: func(fn *Function) struct{} {
				f.formatFunction(fn, depth, next)
				return struct{}{}
			},
			Record: func(r *Record) struct{} {
				f.formatRecord(r, depth, next)
				return struct{}{}
			},
			Literal: func(l *Literal) struct{} {
				f.write(l.String())
				return struct{}{}
			},
			Ref: func(r *Ref) struct{} {
				f.write(r.Name)
				return struct{}{}
			},
			Alias: func(a *Alias) struct{} {
				if a.Name != "" {
					f.write(a.Name)
				} else if a.Target != nil {
					f.formatType(a.Target, depth+1, next)
				} else {
					f.write("alias")
				}
				return struct{}{}
			},
			Platform: func(p *Platform) struct{} {
				f.write(p.Name)
				return struct{}{}
			},
			TypeParam: func(p *TypeParam) struct{} {
				f.write(p.Name)
				if p.Constraint != nil {
					f.write(" : ")
					f.formatType(p.Constraint, depth+1, next)
				}
				return struct{}{}
			},
			Generic: func(g *Generic) struct{} {
				f.write(g.Name)
				f.write("<")
				limit := minInt(len(g.TypeParams), f.opts.MaxTypeParams)
				for i := 0; i < limit; i++ {
					if i > 0 {
						f.write(", ")
					}
					f.formatType(g.TypeParams[i], depth+1, next)
				}
				if limit < len(g.TypeParams) {
					f.write(", ...")
				}
				f.write(">")
				return struct{}{}
			},
			Instantiated: func(i *Instantiated) struct{} {
				if i.Generic != nil && i.Generic.Name != "" {
					f.write(i.Generic.Name)
				} else {
					f.write("inst")
				}
				f.write("<")
				limit := minInt(len(i.TypeArgs), f.opts.MaxTypeParams)
				for idx := 0; idx < limit; idx++ {
					if idx > 0 {
						f.write(", ")
					}
					f.formatType(i.TypeArgs[idx], depth+1, next)
				}
				if limit < len(i.TypeArgs) {
					f.write(", ...")
				}
				f.write(">")
				return struct{}{}
			},
			TypeVar: func(v *TypeVar) struct{} {
				f.write(v.String())
				return struct{}{}
			},
			Interface: func(i *Interface) struct{} {
				if i.Name != "" {
					f.write(i.Name)
				} else {
					f.write("interface{...}")
				}
				return struct{}{}
			},
			Recursive: func(r *Recursive) struct{} {
				f.write(r.String())
				return struct{}{}
			},
			Meta: func(m *Meta) struct{} {
				f.write("typeof(")
				f.formatType(m.Of, depth+1, next)
				f.write(")")
				return struct{}{}
			},
			Default: func(tt Type) struct{} {
				f.write(tt.String())
				return struct{}{}
			},
		}
	})
}

func (f *formatter) formatUnion(u *Union, depth int, guard internal.RecursionGuard) {
	limit := minInt(len(u.Members), f.opts.MaxUnionMembers)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(" | ")
		}
		f.formatType(u.Members[i], depth+1, guard)
	}
	if limit < len(u.Members) {
		f.write(" | ...")
	}
}

func (f *formatter) formatIntersection(u *Intersection, depth int, guard internal.RecursionGuard) {
	limit := minInt(len(u.Members), f.opts.MaxUnionMembers)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(" & ")
		}
		f.formatType(u.Members[i], depth+1, guard)
	}
	if limit < len(u.Members) {
		f.write(" & ...")
	}
}

func (f *formatter) formatTuple(t *Tuple, depth int, guard internal.RecursionGuard) {
	f.write("(")
	limit := minInt(len(t.Elements), f.opts.MaxTupleElems)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		f.formatType(t.Elements[i], depth+1, guard)
	}
	if limit < len(t.Elements) {
		f.write(", ...")
	}
	f.write(")")
}

func (f *formatter) formatFunction(fn *Function, depth int, guard internal.RecursionGuard) {
	f.write("fun")
	if len(fn.TypeParams) > 0 {
		f.write("<")
		limit := minInt(len(fn.TypeParams), f.opts.MaxTypeParams)
		for i := 0; i < limit; i++ {
			if i > 0 {
				f.write(", ")
			}
			f.formatType(fn.TypeParams[i], depth+1, guard)
		}
		if limit < len(fn.TypeParams) {
			f.write(", ...")
		}
		f.write(">")
	}
	f.write("(")
	limit := minInt(len(fn.Params), f.opts.MaxParams)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		param := fn.Params[i]
		if param.Name != "" {
			f.write(param.Name)
			f.write(": ")
		}
		f.formatType(param.Type, depth+1, guard)
		if param.Optional {
			f.write("?")
		}
	}
	if limit < len(fn.Params) {
		f.write(", ...")
	}
	if fn.Variadic != nil {
		if len(fn.Params) > 0 {
			f.write(", ")
		}
		f.write("...")
		f.formatType(fn.Variadic, depth+1, guard)
	}
	f.write(")")

	if len(fn.Returns) > 0 {
		f.write(" -> ")
		if len(fn.Returns) == 1 {
			f.formatType(fn.Returns[0], depth+1, guard)
		} else {
			f.write("(")
			limit = minInt(len(fn.Returns), f.opts.MaxReturns)
			for i := 0; i < limit; i++ {
				if i > 0 {
					f.write(", ")
				}
				f.formatType(fn.Returns[i], depth+1, guard)
			}
			if limit < len(fn.Returns) {
				f.write(", ...")
			}
			f.write(")")
		}
	}
}

func (f *formatter) formatRecord(r *Record, depth int, guard internal.RecursionGuard) {
	f.write("{")
	limit := minInt(len(r.Fields), f.opts.MaxRecordFields)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		field := r.Fields[i]
		if field.Readonly {
			f.write("readonly ")
		}
		f.write(field.Name)
		if field.Optional {
			f.write("?")
		}
		f.write(": ")
		f.formatType(field.Type, depth+1, guard)
	}
	if limit < len(r.Fields) {
		f.write(", ...")
	}
	if r.HasMapComponent() {
		if len(r.Fields) > 0 {
			f.write(", ")
		}
		f.write("[")
		f.formatType(r.MapKey, depth+1, guard)
		f.write("]: ")
		f.formatType(r.MapValue, depth+1, guard)
	}
	if r.Open {
		if len(r.Fields) > 0 || r.HasMapComponent() {
			f.write(", ")
		}
		f.write("...")
	}
	f.write("}")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
