package io

import (
	"bytes"
	"errors"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Manifest file format constants.
const (
	manifestMagic   = 0x4D414E49 // "MANI" - identifies valid manifest files
	manifestVersion = 8          // v8: encode function type params + record open/map components
)

// Manifest decoding errors.
var (
	// ErrInvalidManifest indicates the file header does not contain the magic bytes.
	ErrInvalidManifest = errors.New("invalid manifest header")
	// ErrVersionMismatch indicates the manifest was created with an incompatible version.
	ErrVersionMismatch = errors.New("manifest version mismatch")
)

// Manifest captures type information for cross-module resolution.
//
// A manifest is the persistent representation of a module's type signature,
// enabling type checking across module boundaries without re-analyzing source.
// Manifests are serialized to binary and stored alongside compiled Lua chunks.
//
// The manifest lifecycle:
//  1. Analysis phase generates a manifest from source analysis
//  2. Manifest is serialized and stored (cache or file)
//  3. Dependent modules load the manifest via db.Connect()
//  4. Type checker resolves cross-module references through ManifestQuerier
//
// Thread safety: Manifest instances are immutable after creation. The db.DB
// handles concurrent access during Connect/Disconnect operations.
type Manifest struct {
	// Path is the module path used for require() resolution.
	Path string
	// Version is a monotonic counter for cache invalidation.
	Version uint64

	// Export is the type returned by require(). Usually a record of functions.
	Export typ.Type
	// Types maps type names to their definitions for cross-module type references.
	Types map[string]typ.Type

	// Summaries maps function names to their behavioral specifications.
	// Enables interprocedural analysis without source code.
	Summaries map[string]*FunctionSummary

	// Globals records types assigned to _G for global namespace pollution tracking.
	Globals map[string]typ.Type

	cacheMu             sync.RWMutex
	cachedEnriched      typ.Type
	cachedEnrichedReady bool
	cachedLookupValues  map[string]lookupValueResult
}

type lookupValueResult struct {
	t  typ.Type
	ok bool
}

// ManifestQuerier provides read-only access to module manifests.
//
// This interface decouples manifest consumers from the storage implementation.
// It is implemented by db.DB for the analysis context.
//
// Architecture:
//   - types/io defines Manifest structure and serialization
//   - types/db provides storage and caching via the DB type
//   - Analysis code accesses manifests through ManifestQuerier
//
// This layering prevents import cycles and allows different storage backends.
type ManifestQuerier interface {
	// Manifest returns the manifest for the given module path, or nil if not loaded.
	Manifest(path string) *Manifest
	// Imports returns all loaded manifests keyed by module path.
	Imports() map[string]*Manifest
}

// FunctionSummary captures function behavior for cross-module flow analysis.
//
// Summaries enable sophisticated type checking across module boundaries
// without re-analyzing source code. They encode:
//
// Type information:
//   - Params: Parameter types for arity and type checking
//   - Returns: Return types for result type derivation
//   - Effects: Effect row for effect propagation
//
// Contract information (Hoare-style pre/postconditions):
//   - Requires: Preconditions that must hold at call sites
//   - Ensures: Postconditions guaranteed after the call
//   - ExprRequires/ExprEnsures: Arithmetic constraints on lengths/indices
//
// Escape analysis information:
//   - ParamEscapes: Which parameters escape (are stored, not just borrowed)
//   - ReturnsParam: If >= 0, indicates which param flows to return (passthrough)
//
// The type checker uses summaries to:
//   - Check argument types against parameter types
//   - Derive return types from effects and argument types
//   - Propagate narrowing constraints through function calls
//   - Track value ownership for optimization
type FunctionSummary struct {
	Params  []typ.Type
	Returns []typ.Type
	Effects effect.Row

	// Contract constraints
	Requires     constraint.Condition
	Ensures      constraint.Condition
	ExprRequires []constraint.ExprCompare
	ExprEnsures  []constraint.ExprCompare

	// Escape analysis
	ParamEscapes []bool
	ReturnsParam int // -1 if returns fresh value
}

// Clone returns a deep copy of the FunctionSummary.
func (s *FunctionSummary) Clone() *FunctionSummary {
	if s == nil {
		return nil
	}

	clone := &FunctionSummary{
		Effects:      s.Effects,
		Requires:     cloneCondition(s.Requires),
		Ensures:      cloneCondition(s.Ensures),
		ReturnsParam: s.ReturnsParam,
	}
	if len(s.Params) > 0 {
		clone.Params = make([]typ.Type, len(s.Params))
		copy(clone.Params, s.Params)
	}

	if len(s.Returns) > 0 {
		clone.Returns = make([]typ.Type, len(s.Returns))
		copy(clone.Returns, s.Returns)
	}

	if len(s.ExprRequires) > 0 {
		clone.ExprRequires = make([]constraint.ExprCompare, len(s.ExprRequires))
		copy(clone.ExprRequires, s.ExprRequires)
	}

	if len(s.ExprEnsures) > 0 {
		clone.ExprEnsures = make([]constraint.ExprCompare, len(s.ExprEnsures))
		copy(clone.ExprEnsures, s.ExprEnsures)
	}

	if len(s.ParamEscapes) > 0 {
		clone.ParamEscapes = make([]bool, len(s.ParamEscapes))
		copy(clone.ParamEscapes, s.ParamEscapes)
	}

	return clone
}

func cloneCondition(c constraint.Condition) constraint.Condition {
	if len(c.Disjuncts) == 0 {
		return c
	}
	disjuncts := make([][]constraint.Constraint, len(c.Disjuncts))
	copy(disjuncts, c.Disjuncts)
	return constraint.Condition{Disjuncts: disjuncts}
}

// NewManifest creates an empty manifest.
func NewManifest(path string) *Manifest {
	return &Manifest{
		Path:      path,
		Types:     make(map[string]typ.Type),
		Summaries: make(map[string]*FunctionSummary),
		Globals:   make(map[string]typ.Type),
	}
}

// NewSummary creates a function summary.
func NewSummary(params, returns []typ.Type) *FunctionSummary {
	return &FunctionSummary{
		Params:       params,
		Returns:      returns,
		Effects:      effect.Empty,
		ParamEscapes: make([]bool, len(params)),
		ReturnsParam: -1,
	}
}

// DefineType adds a type definition.
func (m *Manifest) DefineType(name string, t typ.Type) {
	m.Types[name] = t
	m.invalidateCaches()
}

// DefineSummary adds a function summary.
func (m *Manifest) DefineSummary(name string, s *FunctionSummary) {
	m.Summaries[name] = s
	m.invalidateCaches()
}

// SetExport sets the module's export type.
func (m *Manifest) SetExport(t typ.Type) {
	m.Export = t
	m.invalidateCaches()
}

// AddGlobal records a _G assignment.
func (m *Manifest) AddGlobal(name string, t typ.Type) {
	m.Globals[name] = t
}

func (m *Manifest) invalidateCaches() {
	if m == nil {
		return
	}

	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()
	m.cachedEnriched = nil
	m.cachedEnrichedReady = false
	m.cachedLookupValues = nil
}

// LookupType finds a type by name.
func (m *Manifest) LookupType(name string) (typ.Type, bool) {
	t, ok := m.Types[name]
	if !ok {
		return nil, false
	}
	return resolveManifestLocalRefs(t, m.Types), true
}

// AllTypes returns a copy of all type definitions.
func (m *Manifest) AllTypes() map[string]typ.Type {
	if m == nil || len(m.Types) == 0 {
		return nil
	}

	result := make(map[string]typ.Type, len(m.Types))
	for k, v := range m.Types {
		result[k] = v
	}

	return result
}

// AllSummaries returns a deep copy of all function summaries.
func (m *Manifest) AllSummaries() map[string]*FunctionSummary {
	if m == nil || len(m.Summaries) == 0 {
		return nil
	}

	result := make(map[string]*FunctionSummary, len(m.Summaries))
	for k, v := range m.Summaries {
		result[k] = v.Clone()
	}

	return result
}

// AllGlobals returns a copy of all global definitions.
func (m *Manifest) AllGlobals() map[string]typ.Type {
	if m == nil || len(m.Globals) == 0 {
		return nil
	}

	result := make(map[string]typ.Type, len(m.Globals))
	for k, v := range m.Globals {
		result[k] = v
	}

	return result
}

// LookupSummary finds a function summary.
func (m *Manifest) LookupSummary(name string) (*FunctionSummary, bool) {
	if m == nil || name == "" || len(m.Summaries) == 0 {
		return nil, false
	}
	idx := buildSummaryIndex(m.Summaries)
	return idx.lookup(name)
}

// LookupValue finds an exported value field by name from the enriched export type.
func (m *Manifest) LookupValue(name string) (typ.Type, bool) {
	if m == nil || name == "" {
		return nil, false
	}

	m.cacheMu.RLock()
	if m.cachedLookupValues != nil {
		if cached, ok := m.cachedLookupValues[name]; ok {
			m.cacheMu.RUnlock()
			return cached.t, cached.ok
		}
	}
	m.cacheMu.RUnlock()

	var (
		result typ.Type
		ok     bool
	)

	export := unwrap.Alias(m.EnrichedExport())
	switch t := export.(type) {
	case *typ.Record:
		if f := t.GetField(name); f != nil {
			result, ok = f.Type, true
		}
	case *typ.Interface:
		for _, method := range t.Methods {
			if method.Name == name && method.Type != nil {
				result, ok = method.Type, true
				break
			}
		}
	}

	m.cacheMu.Lock()
	if m.cachedLookupValues == nil {
		m.cachedLookupValues = make(map[string]lookupValueResult, 8)
	}
	m.cachedLookupValues[name] = lookupValueResult{t: result, ok: ok}
	m.cacheMu.Unlock()

	return result, ok
}

// EnrichedExport returns the Export type with function summaries applied.
//
// This method combines the structural Export type with behavioral information
// from Summaries. For each function field in the export record, if a matching
// summary exists, the function type is enriched with:
//   - Effects from the summary's effect row
//   - Spec from the summary's contract constraints
//   - Refinement from the summary's ensures condition
//
// This enables cross-module constraint propagation and effect tracking without
// storing redundant information in both Export and Summaries.
func (m *Manifest) EnrichedExport() typ.Type {
	if m == nil {
		return nil
	}

	m.cacheMu.RLock()
	if m.cachedEnrichedReady {
		cached := m.cachedEnriched
		m.cacheMu.RUnlock()
		return cached
	}
	m.cacheMu.RUnlock()

	resolvedExport := resolveManifestLocalRefs(m.Export, m.Types)
	enriched := resolvedExport
	if resolvedExport != nil && len(m.Summaries) > 0 {
		enriched = enrichTypeWithSummaries(resolvedExport, m.Summaries)
	}

	m.cacheMu.Lock()
	if !m.cachedEnrichedReady {
		m.cachedEnriched = enriched
		m.cachedEnrichedReady = true
	}
	cached := m.cachedEnriched
	m.cacheMu.Unlock()

	return cached
}

// resolveManifestLocalRefs resolves local typ.Ref nodes against manifest type
// definitions. This keeps cross-module checks representation-stable by
// materializing named local aliases/records when available.
func resolveManifestLocalRefs(t typ.Type, defs map[string]typ.Type) typ.Type {
	if t == nil || len(defs) == 0 {
		return t
	}
	visiting := make(map[string]bool)

	var resolve func(current typ.Type, depth int) typ.Type
	resolve = func(current typ.Type, depth int) typ.Type {
		if current == nil || typ.DepthExceeded(depth) {
			return current
		}
		return typ.Rewrite(current, func(n typ.Type) (typ.Type, bool) {
			ref, ok := n.(*typ.Ref)
			if !ok || ref.Module != "" {
				return nil, false
			}
			target, exists := defs[ref.Name]
			if !exists || target == nil || visiting[ref.Name] {
				return nil, false
			}
			visiting[ref.Name] = true
			resolved := resolve(target, depth+1)
			delete(visiting, ref.Name)
			if resolved == nil {
				return nil, false
			}
			return resolved, true
		})
	}

	return resolve(t, 0)
}

// enrichTypeWithSummaries applies summaries to function fields in a type.
func enrichTypeWithSummaries(t typ.Type, summaries map[string]*FunctionSummary) typ.Type {
	if t == nil || len(summaries) == 0 {
		return t
	}
	return enrichTopLevelTypeWithSummaries(t, buildSummaryIndex(summaries), 0, make(map[typ.Type]struct{}))
}

func enrichTopLevelTypeWithSummaries(t typ.Type, summaries summaryIndex, depth int, seen map[typ.Type]struct{}) typ.Type {
	if t == nil || len(summaries.exact) == 0 || typ.DepthExceeded(depth) {
		return t
	}
	if seen == nil {
		seen = make(map[typ.Type]struct{})
	}
	if _, ok := seen[t]; ok {
		return t
	}
	seen[t] = struct{}{}
	defer delete(seen, t)

	switch v := t.(type) {
	case *typ.Record:
		return enrichRecordWithSummaries(v, summaries)
	case *typ.Interface:
		return enrichInterfaceWithSummaries(v, summaries)
	case *typ.Alias:
		target := enrichTopLevelTypeWithSummaries(v.Target, summaries, depth+1, seen)
		if target == v.Target {
			return t
		}
		return typ.NewAlias(v.Name, target)
	case *typ.Union:
		changed := false
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			enriched := enrichTopLevelTypeWithSummaries(m, summaries, depth+1, seen)
			members[i] = enriched
			if enriched != m {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return typ.NewUnion(members...)
	case *typ.Intersection:
		changed := false
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			enriched := enrichTopLevelTypeWithSummaries(m, summaries, depth+1, seen)
			members[i] = enriched
			if enriched != m {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return typ.NewIntersection(members...)
	default:
		return t
	}
}

// enrichRecordWithSummaries creates a new record with function fields enriched.
func enrichRecordWithSummaries(r *typ.Record, summaries summaryIndex) *typ.Record {
	if r == nil || len(r.Fields) == 0 {
		return r
	}

	changed := false

	newFields := make([]typ.Field, len(r.Fields))
	for i, f := range r.Fields {
		newFields[i] = f

		if fn, ok := f.Type.(*typ.Function); ok {
			if summary, exists := summaries.lookup(f.Name); exists {
				enriched := ApplyFunctionSummary(fn, summary)
				if enriched != nil && enriched != fn {
					newFields[i].Type = enriched
					changed = true
				}
			}
		}
	}

	if !changed {
		return r
	}

	builder := typ.NewRecord()

	for _, f := range newFields {
		if f.Optional {
			builder.OptField(f.Name, f.Type)
		} else {
			builder.Field(f.Name, f.Type)
		}
	}

	if r.Metatable != nil {
		builder.Metatable(r.Metatable)
	}

	return builder.Build()
}

// enrichInterfaceWithSummaries creates a new interface with method types enriched.
func enrichInterfaceWithSummaries(iface *typ.Interface, summaries summaryIndex) *typ.Interface {
	if iface == nil || len(iface.Methods) == 0 {
		return iface
	}

	changed := false

	newMethods := make([]typ.Method, len(iface.Methods))
	for i, m := range iface.Methods {
		newMethods[i] = m

		if m.Type != nil {
			if summary, exists := summaries.lookup(m.Name); exists {
				enriched := ApplyFunctionSummary(m.Type, summary)
				if enriched != nil && enriched != m.Type {
					newMethods[i].Type = enriched
					changed = true
				}
			}
		}
	}

	if !changed {
		return iface
	}

	return typ.NewInterface(iface.Name, newMethods)
}

// ApplyFunctionSummary enriches a function type with behavioral data from a summary.
//
// This function merges type-level function information with runtime behavioral
// specifications. It is used for cross-module constraint and effect propagation.
//
// Merge strategy:
//   - Effects: Summary effects take precedence if non-empty
//   - Spec: Built from summary constraints if present
//   - Refinement: Built from summary ensures if present
//   - Falls back to existing fn values when summary doesn't provide data
//
// The result is a new function type; the input fn is not modified.
func ApplyFunctionSummary(fn *typ.Function, summary *FunctionSummary) *typ.Function {
	if fn == nil || summary == nil {
		return fn
	}

	// Skip if summary has no useful data
	if summary.Effects.Pure() && !summary.Requires.HasConstraints() && !summary.Ensures.HasConstraints() &&
		len(summary.ExprRequires) == 0 && len(summary.ExprEnsures) == 0 {
		return fn
	}

	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}

	for _, p := range fn.Params {
		if p.Optional {
			builder.OptParam(p.Name, p.Type)
		} else {
			builder.Param(p.Name, p.Type)
		}
	}

	if fn.Variadic != nil {
		builder.Variadic(fn.Variadic)
	}

	builder.Returns(fn.Returns...)

	// Apply effects from summary, fall back to fn's effects
	if !summary.Effects.Pure() {
		builder.Effects(summary.Effects)
	} else if fn.Effects != nil {
		builder.Effects(fn.Effects)
	}

	// Build spec from summary constraints, fall back to fn's spec
	if summary.Requires.HasConstraints() || summary.Ensures.HasConstraints() ||
		len(summary.ExprRequires) > 0 || len(summary.ExprEnsures) > 0 {
		spec := contract.NewSpec()
		spec.Requires = constraint.And(spec.Requires, summary.Requires)
		spec.Ensures = constraint.And(spec.Ensures, summary.Ensures)

		if len(summary.ExprRequires) > 0 {
			spec.WithExprRequires(summary.ExprRequires...)
		}

		if len(summary.ExprEnsures) > 0 {
			spec.WithExprEnsures(summary.ExprEnsures...)
		}

		builder.Spec(spec)
	} else if fn.Spec != nil {
		builder.Spec(fn.Spec)
	}

	// Build refinement from summary ensures (for narrowing), fall back to fn's refinement
	if summary.Ensures.HasConstraints() {
		eff := &constraint.FunctionRefinement{
			OnReturn: summary.Ensures,
		}
		builder.WithRefinement(eff)
	} else if fn.Refinement != nil {
		builder.WithRefinement(fn.Refinement)
	}

	return builder.Build()
}

type summaryIndex struct {
	exact    map[string]*FunctionSummary
	fallback map[string]*FunctionSummary
	ambig    map[string]bool
}

func buildSummaryIndex(summaries map[string]*FunctionSummary) summaryIndex {
	idx := summaryIndex{
		exact:    make(map[string]*FunctionSummary, len(summaries)),
		fallback: make(map[string]*FunctionSummary, len(summaries)),
		ambig:    make(map[string]bool),
	}
	for name, summary := range summaries {
		if name == "" || summary == nil {
			continue
		}
		idx.exact[name] = summary
		canonical := canonicalSummaryName(name)
		if canonical == "" {
			continue
		}
		if existing, ok := idx.fallback[canonical]; ok && existing != summary {
			delete(idx.fallback, canonical)
			idx.ambig[canonical] = true
			continue
		}
		if !idx.ambig[canonical] {
			idx.fallback[canonical] = summary
		}
	}
	return idx
}

func (idx summaryIndex) lookup(name string) (*FunctionSummary, bool) {
	if name == "" {
		return nil, false
	}
	if summary, ok := idx.exact[name]; ok && summary != nil {
		return summary, true
	}
	if idx.ambig[name] {
		return nil, false
	}
	summary, ok := idx.fallback[name]
	if !ok || summary == nil {
		return nil, false
	}
	return summary, true
}

func canonicalSummaryName(name string) string {
	if name == "" {
		return ""
	}
	last := strings.LastIndexAny(name, ".:")
	if last < 0 || last+1 >= len(name) {
		return name
	}
	return name[last+1:]
}

// Encode serializes manifest to binary.
func (m *Manifest) Encode() ([]byte, error) {
	var buf bytes.Buffer
	w := &manifestWriter{typeWriter: &typeWriter{w: &buf}}

	w.writeUint32(manifestMagic)
	w.writeByte(manifestVersion)
	w.writeUint64(m.Version)
	w.writeString(m.Path)

	// Export
	w.writeBool(m.Export != nil)

	if m.Export != nil {
		w.writeType(m.Export)
	}

	// Types
	w.writeUint32(uint32(len(m.Types)))

	for _, name := range sortedKeys(m.Types) {
		w.writeString(name)
		w.writeType(m.Types[name])
	}

	// Summaries
	w.writeUint32(uint32(len(m.Summaries)))

	for _, name := range sortedKeys(m.Summaries) {
		w.writeString(name)
		w.writeSummary(m.Summaries[name])
	}

	// Globals
	w.writeUint32(uint32(len(m.Globals)))

	for _, name := range sortedKeys(m.Globals) {
		w.writeString(name)
		w.writeType(m.Globals[name])
	}

	if w.err != nil {
		return nil, w.err
	}

	return buf.Bytes(), nil
}

// DecodeManifest deserializes binary to manifest.
func DecodeManifest(data []byte) (*Manifest, error) {
	r := &manifestReader{typeReader: &typeReader{r: bytes.NewReader(data)}}

	if r.readUint32() != manifestMagic {
		return nil, ErrInvalidManifest
	}

	if r.readByte() != manifestVersion {
		return nil, ErrVersionMismatch
	}

	m := &Manifest{
		Version:   r.readUint64(),
		Path:      r.readString(),
		Types:     make(map[string]typ.Type),
		Summaries: make(map[string]*FunctionSummary),
		Globals:   make(map[string]typ.Type),
	}

	// Export
	if r.readBool() {
		m.Export = r.readType()
	}

	// Types
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return nil, r.err
	}
	for i := uint32(0); i < count; i++ {
		name := r.readString()
		m.Types[name] = r.readType()
	}

	// Summaries
	count = r.readUint32()
	if !r.checkSliceLen(count) {
		return nil, r.err
	}
	for i := uint32(0); i < count; i++ {
		name := r.readString()
		m.Summaries[name] = r.readSummary()
	}

	// Globals
	count = r.readUint32()
	if !r.checkSliceLen(count) {
		return nil, r.err
	}
	for i := uint32(0); i < count; i++ {
		name := r.readString()
		m.Globals[name] = r.readType()
	}

	if r.err != nil {
		return nil, r.err
	}

	return m, nil
}

// EncodeManifest encodes a manifest to bytes.
func EncodeManifest(m *Manifest) ([]byte, error) {
	return m.Encode()
}

type manifestWriter struct {
	*typeWriter
}

func (w *manifestWriter) writeSummary(s *FunctionSummary) {
	// Params
	w.writeUint32(uint32(len(s.Params)))

	for _, p := range s.Params {
		w.writeType(p)
	}

	// Returns
	w.writeUint32(uint32(len(s.Returns)))

	for _, r := range s.Returns {
		w.writeType(r)
	}

	// Effects
	w.writeEffectRow(s.Effects)

	// Contract
	w.writeCondition(s.Requires)
	w.writeCondition(s.Ensures)
	w.writeExprCompares(s.ExprRequires)
	w.writeExprCompares(s.ExprEnsures)

	// Escape
	w.writeUint32(uint32(len(s.ParamEscapes)))

	for _, esc := range s.ParamEscapes {
		w.writeBool(esc)
	}

	w.writeUint32(uint32(s.ReturnsParam + 1)) // +1 to handle -1
}

type manifestReader struct {
	*typeReader
}

func (r *manifestReader) readSummary() *FunctionSummary {
	s := &FunctionSummary{}

	// Params
	count := r.readUint32()
	if !r.checkSliceLen(count) {
		return s
	}

	s.Params = make([]typ.Type, count)
	for i := uint32(0); i < count; i++ {
		s.Params[i] = r.readType()
	}

	// Returns
	count = r.readUint32()
	if !r.checkSliceLen(count) {
		return s
	}

	s.Returns = make([]typ.Type, count)
	for i := uint32(0); i < count; i++ {
		s.Returns[i] = r.readType()
	}

	// Effects
	s.Effects = r.readEffectRow()

	// Contract
	s.Requires = r.readCondition()
	s.Ensures = r.readCondition()
	s.ExprRequires = r.readExprCompares()
	s.ExprEnsures = r.readExprCompares()

	// Escape
	count = r.readUint32()
	if !r.checkSliceLen(count) {
		return s
	}

	s.ParamEscapes = make([]bool, count)
	for i := uint32(0); i < count; i++ {
		s.ParamEscapes[i] = r.readBool()
	}

	s.ReturnsParam = int(r.readUint32()) - 1

	return s
}
