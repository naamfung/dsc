package intercept

import "github.com/wippyai/go-lua/types/io"

// ChainBuilder provides a fluent API for constructing intercept chains.
//
// Standard chain order:
// - Call intercepts: select() -> require() -> type casts
// - Method intercepts: Type:is()
//
// Usage:
//
//	chain := NewChainBuilder().
//	    WithManifests(manifests).
//	    WithVariadicResolver(resolver).
//	    Build()
type ChainBuilder struct {
	manifests        io.ManifestQuerier
	variadicResolver VariadicTypeResolver
}

// NewChainBuilder creates a new builder for intercept chains.
func NewChainBuilder() *ChainBuilder {
	return &ChainBuilder{}
}

// WithManifests sets the manifest querier for require() interception.
func (b *ChainBuilder) WithManifests(m io.ManifestQuerier) *ChainBuilder {
	b.manifests = m
	return b
}

// WithVariadicResolver sets the variadic resolver for select() interception.
func (b *ChainBuilder) WithVariadicResolver(v VariadicTypeResolver) *ChainBuilder {
	b.variadicResolver = v
	return b
}

// Build creates the intercept chain with standard order:
// Call intercepts: 1. select, 2. require, 3. typecast
// Method intercepts: 1. type:is
func (b *ChainBuilder) Build() *Chain {
	callIntercepts := []CallIntercept{
		&SelectIntercept{VariadicResolver: b.variadicResolver},
		&RequireIntercept{Manifests: b.manifests},
		&TypeCastIntercept{},
	}

	methodIntercepts := []MethodIntercept{
		&TypeIsIntercept{},
	}

	return NewChain(callIntercepts, methodIntercepts)
}
