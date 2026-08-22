package typ

import (
	"strings"

	"github.com/wippyai/go-lua/types/kind"
)

// Param represents a function parameter with name, type, and optionality.
type Param struct {
	Name     string
	Type     Type
	Optional bool // True if parameter has a default value
}

// Function represents a function type with parameters, returns, and effects.
//
// Functions support generics via TypeParams, variadic arguments via Variadic,
// multiple return values via Returns, and effect tracking via Effects.
//
// The Spec field holds Hoare-style contracts (pre/post conditions).
// The Refinement field holds type narrowing constraints for predicate functions.
type Function struct {
	TypeParams   []*TypeParam   // Generic type parameters (empty for non-generic)
	Params       []Param        // Positional parameters
	Variadic     Type           // Variadic element type (nil if not variadic)
	Returns      []Type         // Return types (empty for void functions)
	Effects      EffectInfo     // Effect row (effect.Row) for mutation/throw/io tracking
	Spec         SpecInfo       // Contract specification (*contract.Spec)
	Refinement   RefinementInfo // Type refinement effect (*constraint.FunctionRefinement)
	hash         uint64
	softPrunable bool
	strCache     stringCache
}

// FunctionBuilder provides a fluent API for constructing function types.
//
// Example:
//
//	fn := typ.Func().
//	    Param("x", typ.Number).
//	    Param("y", typ.Number).
//	    Returns(typ.Number).
//	    Build()
type FunctionBuilder struct {
	typeParams []*TypeParam
	params     []Param
	variadic   Type
	returns    []Type
	effects    EffectInfo
	spec       SpecInfo
	refinement RefinementInfo
}

// Func starts building a function type.
func Func() *FunctionBuilder {
	return &FunctionBuilder{}
}

// TypeParam adds a type parameter for generic functions.
func (b *FunctionBuilder) TypeParam(name string, constraint Type) *FunctionBuilder {
	b.typeParams = append(b.typeParams, NewTypeParam(name, constraint))
	return b
}

// Param adds a required parameter.
func (b *FunctionBuilder) Param(name string, t Type) *FunctionBuilder {
	b.params = append(b.params, Param{Name: name, Type: t})
	return b
}

// OptParam adds an optional parameter.
func (b *FunctionBuilder) OptParam(name string, t Type) *FunctionBuilder {
	b.params = append(b.params, Param{Name: name, Type: t, Optional: true})
	return b
}

// Variadic sets variadic parameter type.
func (b *FunctionBuilder) Variadic(t Type) *FunctionBuilder {
	b.variadic = t
	return b
}

// Returns sets return types.
func (b *FunctionBuilder) Returns(types ...Type) *FunctionBuilder {
	b.returns = types
	return b
}

// Effects sets effect row.
func (b *FunctionBuilder) Effects(e EffectInfo) *FunctionBuilder {
	b.effects = e
	return b
}

// Spec sets contract specification.
func (b *FunctionBuilder) Spec(s SpecInfo) *FunctionBuilder {
	b.spec = s
	return b
}

// WithRefinement sets the function refinement effect.
func (b *FunctionBuilder) WithRefinement(r RefinementInfo) *FunctionBuilder {
	b.refinement = r
	return b
}

// Build creates the function type.
func (b *FunctionBuilder) Build() *Function {
	return buildFunctionType(
		b.typeParams,
		b.params,
		b.variadic,
		b.returns,
		b.effects,
		b.spec,
		b.refinement,
	)
}

func (f *Function) Kind() kind.Kind { return kind.Function }

func (f *Function) String() string {
	return f.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString("fun")

		if len(f.TypeParams) > 0 {
			sb.WriteString("<")

			for i, tp := range f.TypeParams {
				if i > 0 {
					sb.WriteString(", ")
				}

				sb.WriteString(tp.String())
			}

			sb.WriteString(">")
		}

		sb.WriteString("(")

		for i, p := range f.Params {
			if i > 0 {
				sb.WriteString(", ")
			}

			if p.Name != "" {
				sb.WriteString(p.Name)
				sb.WriteString(": ")
			}

			sb.WriteString(p.Type.String())

			if p.Optional {
				sb.WriteString("?")
			}
		}

		if f.Variadic != nil {
			if len(f.Params) > 0 {
				sb.WriteString(", ")
			}

			sb.WriteString("...")
			sb.WriteString(f.Variadic.String())
		}

		sb.WriteString(")")

		if len(f.Returns) > 0 {
			sb.WriteString(" -> ")

			if len(f.Returns) == 1 {
				sb.WriteString(f.Returns[0].String())
			} else {
				sb.WriteString("(")

				for i, r := range f.Returns {
					if i > 0 {
						sb.WriteString(", ")
					}

					sb.WriteString(r.String())
				}

				sb.WriteString(")")
			}
		}

		return sb.String()
	})
}

func (f *Function) Hash() uint64 { return f.hash }

func (f *Function) Equals(other Type) bool {
	return TypeEquals(f, other)
}
