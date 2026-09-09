// transfer.go implements the transfer functions for the flow analysis worklist algorithm.
//
// Transfer functions define how types change at each CFG point. This file handles:
//
//   - Phi node processing: Joining types from multiple incoming branches
//   - Assignment processing: Computing types for assignment targets
//   - Iterator derivation: Computing loop variable types from iterator sources
//   - Widening: Expanding types for table mutations and dynamic indexing
//
// The functions return changed keys to drive worklist propagation.
package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/join"
	"github.com/wippyai/go-lua/types/flow/pathkey"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// processPointReturnChangedKeys processes all type-changing operations at a CFG point.
//
// This is the main transfer function called by the worklist algorithm. It processes:
//   - Phi nodes: Joining types from multiple control flow paths
//   - Assignments: Computing and storing target types
//
// Returns the list of canonical keys whose types changed, used to determine
// which dependent points need re-processing.
func (s *Solution) processPointReturnChangedKeys(p cfg.Point) []string {
	c := s.inputs.Graph
	node := c.Node(p)
	if node == nil {
		return nil
	}

	var changedKeys []string

	// Process phi nodes at this point
	phiKeys := s.processJoinReturnChangedKeys(p)
	changedKeys = append(changedKeys, phiKeys...)

	assignKeys := s.processAssignmentReturnChangedKeys(p)
	changedKeys = append(changedKeys, assignKeys...)

	return changedKeys
}

// processAssignmentReturnChangedKeys processes all assignments at a CFG point.
//
// For each assignment targeting this point, computes the assigned type by:
//  1. Looking up source path type via NarrowedTypeAt (for flow propagation)
//  2. Deriving iterator variable types from IteratorSource
//  3. Deriving container element types from ContainerElementSource
//  4. Falling back to the statically extracted type
//
// Also handles special assignment types:
//   - IndexerAssignments: Dynamic index (t[k] = v) widening empty tables to maps
//   - TableMutatorAssignments: table.insert-like array element widening
//   - ContainerMutatorAssignments: channel.send-like element type widening
//
// Returns keys that changed, enabling worklist-driven convergence.
func (s *Solution) processAssignmentReturnChangedKeys(p cfg.Point) []string {
	var changedKeys []string

	for _, assign := range s.inputs.Assignments {
		if assign.Point != p {
			continue
		}

		// Field/index writes create a new symbol version. Carry forward unchanged
		// base/suffix facts from predecessor versions so sibling fields remain stable.
		if len(assign.TargetPath.Segments) > 0 {
			changedKeys = append(changedKeys, s.carryForwardStructuredVersionFacts(p, assign.TargetPath)...)
		}

		var assignedType typ.Type
		if assign.SourcePath.HasSymbol() {
			assignedType = s.assignmentSourceTypeAt(p, assign)
		}
		if assign.IterSource != nil {
			if iterType := s.iterVarTypeAt(p, assign.IterSource); iterType != nil {
				assignedType = iterType
			}
		}
		// Derive type from container element type if ContainerElementSource is set
		if assign.ContainerElementSource != nil {
			if elemType := s.containerElementTypeAt(p, assign.ContainerElementSource); elemType != nil {
				assignedType = elemType
			}
		}
		// Derive type from dynamic map index read if MapElementSource is set.
		if assign.MapElementSource != nil {
			if elemType := s.mapElementTypeAt(p, assign.MapElementSource); elemType != nil {
				assignedType = elemType
			}
		}
		if assignedType == nil {
			assignedType = assign.Type
		}

		targetKey := s.pkResolver.KeyAt(p, assign.TargetPath)
		if targetKey == "" {
			continue
		}

		// Track alias provenance for path assignments: t.cur = s
		// so writes through t.cur.* can be mirrored to s.*.
		if len(assign.TargetPath.Segments) > 0 {
			targetKeyStr := string(targetKey)
			if assign.SourcePath.HasSymbol() {
				sourceKey := s.pkResolver.KeyAt(p, assign.SourcePath)
				if sourceKey != "" && sourceKey != targetKey {
					s.pathAliases[targetKeyStr] = string(sourceKey)
				} else {
					delete(s.pathAliases, targetKeyStr)
				}
			} else {
				delete(s.pathAliases, targetKeyStr)
			}
		}

		if assignedType != nil {
			targetKeyStr := string(targetKey)
			old := s.values[targetKeyStr]
			if len(assign.TargetPath.Segments) > 0 && assignedType.Kind() == kind.Nil {
				assignedType = s.normalizeNilFieldAssignmentType(p, assign.TargetPath, old)
			}
			if !typ.TypeEquals(old, assignedType) {
				s.setValue(targetKeyStr, assignedType)
				changedKeys = append(changedKeys, targetKeyStr)
			}
			if len(assign.TargetPath.Segments) > 0 {
				changedKeys = append(changedKeys, s.mirrorAliasedFieldWrite(p, assign.TargetPath, assignedType)...)
			}
		}
	}

	// Process indexer assignments to widen tables
	for _, ia := range s.inputs.IndexerAssignments {
		if ia.Point != p {
			continue
		}
		if key := s.processIndexerAssignmentReturnKey(p, ia); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process table mutator assignments (table.insert-like)
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.Point != p {
			continue
		}
		if key := s.processTableMutatorAssignmentReturnKey(p, tm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process container mutator assignments (channel.send-like)
	for _, cm := range s.inputs.ContainerMutatorAssignments {
		if cm.Point != p {
			continue
		}
		if key := s.processContainerMutatorAssignmentReturnKey(p, cm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	return changedKeys
}

func (s *Solution) assignmentSourceTypeAt(p cfg.Point, assign UnifiedAssignment) typ.Type {
	if !assign.SourcePath.HasSymbol() {
		return nil
	}
	// Lua evaluates RHS before writing assignment targets. For self-related
	// assignments, source reads must come from predecessor state.
	if s.assignmentNeedsPreState(assign) {
		if pre := s.preAssignmentNarrowedTypeAt(p, assign.SourcePath); pre != nil {
			return pre
		}
	}
	return s.NarrowedTypeAt(p, assign.SourcePath)
}

func (s *Solution) assignmentNeedsPreState(assign UnifiedAssignment) bool {
	if assign.TargetPath.Symbol == 0 || assign.SourcePath.Symbol == 0 {
		return false
	}
	if assign.TargetPath.Symbol != assign.SourcePath.Symbol {
		return false
	}
	return pathkey.PathRelated(assign.TargetPath, assign.SourcePath)
}

func (s *Solution) preAssignmentNarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || path.Symbol == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var joined []typ.Type
	seenVersions := make(map[int]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.ID == 0 {
			continue
		}
		if _, seen := seenVersions[ver.ID]; seen {
			continue
		}
		seenVersions[ver.ID] = struct{}{}

		predPath := path
		predPath.Version = ver.ID
		if t := s.NarrowedTypeAt(pred, predPath); t != nil {
			joined = append(joined, t)
		}
	}
	if len(joined) == 0 {
		return nil
	}
	return join.Types(joined...)
}

func (s *Solution) mirrorAliasedFieldWrite(p cfg.Point, targetPath constraint.Path, assignedType typ.Type) []string {
	if s == nil || s.pkResolver == nil || assignedType == nil || targetPath.Symbol == 0 {
		return nil
	}
	if len(targetPath.Segments) == 0 {
		return nil
	}

	// Find the longest aliased prefix, e.g. for t.cur.hook use alias(t.cur) => s.
	for cut := len(targetPath.Segments) - 1; cut >= 1; cut-- {
		prefix := constraint.Path{
			Root:     targetPath.Root,
			Symbol:   targetPath.Symbol,
			Segments: targetPath.Segments[:cut],
		}
		sourceKey := s.aliasSourceKeyAt(p, prefix)
		if sourceKey == "" {
			continue
		}

		sourceSym, sourceVersion, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(sourceKey))
		if !ok || sourceSym == 0 || sourceVersion == 0 {
			continue
		}

		aliasSegs := pathkey.ParseSuffix(suffix)
		destSegs := append(append([]constraint.Segment{}, aliasSegs...), targetPath.Segments[cut:]...)
		destKey := s.pkResolver.KeyAtVersion(sourceSym, sourceVersion, destSegs)
		if destKey == "" {
			return nil
		}

		keyStr := string(destKey)
		old := s.values[keyStr]
		newType := assignedType
		if len(destSegs) > 0 && assignedType.Kind() == kind.Nil {
			newType = s.normalizeNilFieldAssignmentType(p, constraint.Path{
				Symbol:   sourceSym,
				Version:  sourceVersion,
				Segments: destSegs,
			}, old)
		}
		if typ.TypeEquals(old, newType) {
			return nil
		}
		s.setValue(keyStr, newType)
		return []string{keyStr}
	}

	return nil
}

func (s *Solution) aliasSourceKeyAt(p cfg.Point, path constraint.Path) string {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil || s.pathAliases == nil {
		return ""
	}
	if path.Symbol == 0 || path.IsEmpty() {
		return ""
	}

	// First try alias captured at current version.
	currentKey := s.pkResolver.KeyAt(p, path)
	if currentKey != "" {
		if source, ok := s.pathAliases[string(currentKey)]; ok {
			return source
		}
	}

	// If the root is directly reassigned at this point, do not inherit predecessor aliases.
	if s.hasRootAssignmentAtPoint(p, path.Symbol) {
		return ""
	}

	// Fall back to predecessor versions for carried structured aliases.
	for _, pred := range graphPredecessors(s.inputs.Graph, p) {
		ver := s.inputs.Graph.VisibleVersion(pred, path.Symbol)
		if ver.Symbol == 0 || ver.ID == 0 {
			continue
		}
		predKey := s.pkResolver.KeyAtVersion(path.Symbol, ver.ID, path.Segments)
		if predKey == "" {
			continue
		}
		if source, ok := s.pathAliases[string(predKey)]; ok {
			return source
		}
	}
	return ""
}

func (s *Solution) hasRootAssignmentAtPoint(p cfg.Point, sym cfg.SymbolID) bool {
	if s == nil || s.inputs == nil || sym == 0 {
		return false
	}
	for _, assign := range s.inputs.Assignments {
		if assign.Point != p {
			continue
		}
		if assign.TargetPath.Symbol != sym {
			continue
		}
		if len(assign.TargetPath.Segments) == 0 {
			return true
		}
	}
	return false
}

// carryForwardStructuredVersionFacts seeds the current version with predecessor
// root/suffix facts for structured values when assigning through a sub-path.
func (s *Solution) carryForwardStructuredVersionFacts(p cfg.Point, targetPath constraint.Path) []string {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil {
		return nil
	}
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return nil
	}

	currentBase := constraint.Path{
		Root:    targetPath.Root,
		Symbol:  targetPath.Symbol,
		Version: targetPath.Version,
	}
	currentBaseKey := s.pkResolver.KeyAt(p, currentBase)
	if currentBaseKey == "" {
		return nil
	}

	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	predBaseKeys := make([]string, 0, len(preds))
	seenPredBase := make(map[string]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, targetPath.Symbol)
		if ver.Symbol == 0 || ver.ID == 0 {
			continue
		}
		predBaseKey := s.pkResolver.KeyAtVersion(ver.Symbol, ver.ID, nil)
		if predBaseKey == "" {
			continue
		}
		key := string(predBaseKey)
		if _, seen := seenPredBase[key]; seen {
			continue
		}
		seenPredBase[key] = struct{}{}
		predBaseKeys = append(predBaseKeys, key)
	}
	if len(predBaseKeys) == 0 {
		return nil
	}

	var changedKeys []string
	currentBaseKeyStr := string(currentBaseKey)

	// Seed root/base value if missing on current version.
	if s.values[currentBaseKeyStr] == nil {
		baseTypes := make([]typ.Type, 0, len(predBaseKeys))
		for _, predBaseKey := range predBaseKeys {
			if t := s.values[predBaseKey]; t != nil {
				baseTypes = append(baseTypes, t)
			}
		}
		if len(baseTypes) > 0 {
			joinedBase := join.Types(baseTypes...)
			if !typ.TypeEquals(s.values[currentBaseKeyStr], joinedBase) {
				s.values[currentBaseKeyStr] = joinedBase
				changedKeys = append(changedKeys, currentBaseKeyStr)
			}
		}
	}

	// Seed suffix values from predecessor versions when missing on current version.
	suffixTypes := make(map[string][]typ.Type)
	for _, predBaseKey := range predBaseKeys {
		prefixLen := len(predBaseKey)
		for key, t := range s.values {
			if t == nil || len(key) <= prefixLen || key[:prefixLen] != predBaseKey {
				continue
			}
			suffix := key[prefixLen:]
			if len(suffix) == 0 || (suffix[0] != '.' && suffix[0] != '[') {
				continue
			}
			suffixTypes[suffix] = append(suffixTypes[suffix], t)
		}
	}

	for suffix, types := range suffixTypes {
		if len(types) == 0 {
			continue
		}
		key := currentBaseKeyStr + suffix
		if s.values[key] != nil {
			continue
		}
		joined := join.Types(types...)
		if !typ.TypeEquals(s.values[key], joined) {
			s.values[key] = joined
			changedKeys = append(changedKeys, key)
		}
	}

	return changedKeys
}

func (s *Solution) normalizeNilFieldAssignmentType(p cfg.Point, targetPath constraint.Path, old typ.Type) typ.Type {
	if !typ.IsAbsentOrUnknown(old) {
		return typ.JoinPreferNonSoft(old, typ.Nil)
	}
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return typ.Nil
	}

	rootPath := constraint.Path{
		Root:    targetPath.Root,
		Symbol:  targetPath.Symbol,
		Version: targetPath.Version,
	}
	rootType := s.TypeAt(p, rootPath)
	if rootType == nil {
		return typ.Nil
	}
	derived, ok := s.deriveTypeFrom(rootType, targetPath.Segments)
	if !ok || typ.IsAbsentOrUnknown(derived) {
		return typ.Nil
	}
	return typ.JoinPreferNonSoft(derived, typ.Nil)
}

// iterVarTypeAt derives iterator variable type from the iterator source at point p.
//
// For Lua's iteration constructs (for-in loops), variable types depend on the
// iteration kind and variable position:
//
//   - IterateIndexed (ipairs): Index is always integer, value is array element
//   - IterateKeyed (pairs): Key is table key type, value is table value type
//
// Special handling for keys-provenance variables: When iterating over a variable
// that holds sorted keys of another table (via sorted_keys() or similar), the
// element type is derived from the original table's key type rather than the
// intermediate array.
//
// Falls back to declared type when flow type is missing or imprecise, preserving
// annotated element types through widening operations.
func (s *Solution) iterVarTypeAt(p cfg.Point, iter *IteratorSource) typ.Type {
	if iter == nil || iter.Path.IsEmpty() {
		return nil
	}

	// Index variable for ipairs-style iteration is always integer.
	if iter.Kind == IterateIndexed && iter.VarIndex == 0 {
		return typ.Integer
	}

	srcType := s.NarrowedTypeAt(p, iter.Path)
	if srcType == nil || srcType.Kind().IsPlaceholder() {
		if preType := s.preAssignmentNarrowedTypeAt(p, iter.Path); preType != nil {
			// Prefer predecessor flow evidence over placeholder/absent current state.
			if srcType == nil || srcType.Kind().IsPlaceholder() || !preType.Kind().IsPlaceholder() {
				srcType = preType
			}
		}
	}

	// For indexed iteration (ipairs) over keys-provenance variables,
	// reconcile the original table key type with the keys-array element type.
	// Pattern: local keys = sorted_keys(t); for _, k in ipairs(keys) do
	if iter.Kind == IterateIndexed && iter.VarIndex == 1 {
		if origTableSym := s.keysProvenanceSource(iter.Path.Symbol); origTableSym != 0 {
			var sourceElem typ.Type
			if srcType != nil {
				sourceElem = s.elementTypeForIter(srcType, iter)
			}
			if sourceElem == nil {
				if declType := s.lookupDeclaredType(iter.Path); declType != nil {
					sourceElem = s.elementTypeForIter(declType, iter)
				}
			}
			origPath := constraint.Path{Root: s.inputs.Graph.NameOf(origTableSym), Symbol: origTableSym}
			if origType := s.NarrowedTypeAt(p, origPath); origType != nil {
				if keyType := s.inputs.Decomposer.KeyType(origType); keyType != nil {
					if merged := mergeKeysProvenanceElemType(sourceElem, keyType); merged != nil {
						return merged
					}
				}
			}
		}
	}

	// When flow type is missing or imprecise, prefer the declared type
	// so that annotated element types are preserved through widening.
	if srcType == nil || srcType.Kind().IsPlaceholder() {
		if declType := s.lookupDeclaredType(iter.Path); declType != nil {
			if elem := s.elementTypeForIter(declType, iter); elem != nil {
				return elem
			}
			if declType.Kind().IsPlaceholder() {
				return typ.Any
			}
		}
	}

	if srcType == nil {
		return nil
	}

	if elem := s.elementTypeForIter(srcType, iter); elem != nil {
		return elem
	}
	if srcType.Kind().IsPlaceholder() {
		return typ.Any
	}
	return nil
}

func mergeKeysProvenanceElemType(elemType, keyType typ.Type) typ.Type {
	if elemType == nil {
		return keyType
	}
	if keyType == nil {
		return elemType
	}
	// Placeholder key domains (any/unknown) add no usable precision.
	if keyType.Kind().IsPlaceholder() {
		return elemType
	}
	// If the keys array element is still placeholder, use provenance.
	if elemType.Kind().IsPlaceholder() {
		return keyType
	}
	// Prefer the more specific type when one subsumes the other.
	if subtype.IsSubtype(elemType, keyType) {
		return elemType
	}
	if subtype.IsSubtype(keyType, elemType) {
		return keyType
	}
	// Reconcile independent evidence; if incompatible, keep source-table key type.
	intersection := subtype.NormalizeIntersection(elemType, keyType)
	if intersection != nil && !typ.IsNever(intersection) {
		return intersection
	}
	return keyType
}

// keysProvenanceSource returns the original table symbol if sym holds keys of that table.
//
// Some patterns extract keys from a table into an array (e.g., local keys = sorted_keys(t)).
// When iterating over such an array, the element type should be the original table's key type,
// not simply string or the array's element type. This method looks up the provenance chain
// to find the original table.
func (s *Solution) keysProvenanceSource(sym cfg.SymbolID) cfg.SymbolID {
	if sym == 0 || s.inputs.KeysProvenance == nil {
		return 0
	}
	return s.inputs.KeysProvenance[sym]
}

// elementTypeForIter extracts the appropriate element/key/value type.
//
// The extraction depends on iterator kind and variable position:
//
//	IterateIndexed (ipairs):
//	  VarIndex 0: Always integer (handled elsewhere)
//	  VarIndex 1: ElementType (array element)
//
//	IterateKeyed (pairs):
//	  VarIndex 0: KeyType (map/record key)
//	  VarIndex 1: ValueType (map/record value)
//
// Uses TypeDecomposer for extraction to support various container types.
func (s *Solution) elementTypeForIter(srcType typ.Type, iter *IteratorSource) typ.Type {
	d := s.inputs.Decomposer
	switch iter.Kind {
	case IterateIndexed:
		if iter.VarIndex == 1 {
			return d.ElementType(srcType)
		}
	case IterateKeyed:
		switch iter.VarIndex {
		case 0:
			return d.KeyType(srcType)
		case 1:
			return d.ValueType(srcType)
		}
	}
	return nil
}

// containerElementTypeAt derives the element type from a container at point p.
//
// Used for methods like channel:receive() where the return type depends on the
// container's element type at the call site. The element type is derived from:
//
//  1. Instantiated generics (e.g., Channel<T>): Uses first type argument
//  2. Arrays: Uses ElementType via decomposer
//  3. Other containers: Falls back to decomposer.ElementType
//
// This enables accurate return type inference for generic container methods
// even when the container was widened during flow analysis.
func (s *Solution) containerElementTypeAt(p cfg.Point, src *ContainerElementSource) typ.Type {
	if src == nil || src.ContainerPath.IsEmpty() {
		return nil
	}

	containerType := s.NarrowedTypeAt(p, src.ContainerPath)
	if containerType == nil {
		return nil
	}

	// Extract element type from instantiated generics (e.g., Channel<T>)
	if inst, ok := containerType.(*typ.Instantiated); ok {
		if len(inst.TypeArgs) > 0 {
			return inst.TypeArgs[0]
		}
	}

	// Fall back to ElementType for arrays and other containers
	return s.inputs.Decomposer.ElementType(containerType)
}

// mapElementTypeAt derives the value type from a dynamic map index read source.
//
// For assignments like `local v = t[k]` where the key is non-const, extraction
// records MapElementSource so solve-time flow can derive `v` from current map type.
func (s *Solution) mapElementTypeAt(p cfg.Point, src *MapElementSource) typ.Type {
	if src == nil || src.MapPath.IsEmpty() {
		return nil
	}

	mapType := s.NarrowedTypeAt(p, src.MapPath)
	if mapType == nil || mapType.Kind().IsPlaceholder() {
		if preType := s.preAssignmentNarrowedTypeAt(p, src.MapPath); preType != nil {
			// Prefer predecessor flow evidence over placeholder/absent current state.
			if mapType == nil || mapType.Kind().IsPlaceholder() || !preType.Kind().IsPlaceholder() {
				mapType = preType
			}
		}
	}
	if mapType == nil {
		if declType := s.lookupDeclaredType(src.MapPath); declType != nil {
			mapType = declType
		}
	}
	if (mapType == nil || mapType.Kind().IsPlaceholder() || isEmptyRecordNoMapType(mapType)) &&
		src.MapPath.Symbol != 0 && len(src.MapPath.Segments) == 0 {
		if known := s.joinKnownRootTypes(src.MapPath.Symbol); known != nil {
			if mapType == nil {
				mapType = known
			} else {
				mapType = join.Types(mapType, known)
			}
		}
	}
	if mapType == nil {
		return nil
	}

	if valueType := s.inputs.Decomposer.ValueType(mapType); valueType != nil {
		return valueType
	}
	if mapType.Kind().IsPlaceholder() {
		return typ.Any
	}
	// Dynamic index on known container shape with no value evidence resolves to nil.
	// This preserves Lua table semantics for missing keys and avoids placeholder
	// fallback poisoning in loop fixpoint before map writes are observed.
	return typ.Nil
}

// processIndexerAssignmentReturnKey handles dynamic index assignments like t[k] = v.
//
// When a table is indexed with a non-constant key (t[k] = v), the table may need
// widening to accommodate dynamic keys. This method:
//
//  1. Skips annotated variables (explicit types are preserved)
//  2. Resolves key type from flow state or explicit override
//  3. Calls widenWithIndexer to expand the table type
//
// Widening rules:
//   - Empty record {} becomes map {[K]: V}
//   - Record with fields gains a map component
//   - Existing map widens key/value types via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processIndexerAssignmentReturnKey(p cfg.Point, ia IndexerAssignment) string {
	if ia.Symbol == 0 {
		return ""
	}

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[ia.Symbol] {
			return ""
		}
	}

	// Resolve key type from flow state or explicit override
	keyType := ia.KeyType
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		keyType = s.resolveSymbolKeyType(p, ia.KeySymbol, ia.KeyVar)
	}
	keyType = normalizeDynamicKeyType(keyType)

	// Resolve value type from flow state or use fallback.
	valueType := ia.ValType
	if ia.ValuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, ia.ValuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if valueType == nil {
		return ""
	}

	// Get canonical key for the root variable
	iaPath := constraint.Path{Root: ia.Root, Symbol: ia.Symbol, Segments: ia.Segments}
	pathKey := s.pkResolver.KeyAt(p, iaPath)
	if pathKey == "" {
		return ""
	}

	// Get current type of the root
	currentType := s.values[string(pathKey)]
	if currentType == nil {
		currentType = s.joinPredecessorRootTypes(p, ia.Symbol)
	}
	currentType = preferDeclaredTemplateForWiden(currentType, s.lookupDeclaredType(constraint.Path{
		Root:   ia.Root,
		Symbol: ia.Symbol,
	}))

	// Compute the widened type
	newType := widenWithIndexer(currentType, keyType, valueType)
	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.setValue(string(pathKey), newType)
	return string(pathKey)
}

func (s *Solution) joinPredecessorRootTypes(p cfg.Point, sym cfg.SymbolID) typ.Type {
	if s == nil || s.inputs == nil || s.inputs.Graph == nil || s.pkResolver == nil || sym == 0 {
		return nil
	}
	preds := graphPredecessors(s.inputs.Graph, p)
	if len(preds) == 0 {
		return nil
	}

	var types []typ.Type
	seen := make(map[string]struct{}, len(preds))
	for _, pred := range preds {
		ver := s.inputs.Graph.VisibleVersion(pred, sym)
		if ver.Symbol == 0 || ver.ID == 0 {
			continue
		}
		key := s.pkResolver.KeyAtVersion(ver.Symbol, ver.ID, nil)
		if key == "" {
			continue
		}
		keyStr := string(key)
		if _, ok := seen[keyStr]; ok {
			continue
		}
		seen[keyStr] = struct{}{}
		if t := s.values[keyStr]; t != nil {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		return nil
	}
	return join.Types(types...)
}

func (s *Solution) joinKnownRootTypes(sym cfg.SymbolID) typ.Type {
	if s == nil || s.pkResolver == nil || sym == 0 {
		return nil
	}

	var types []typ.Type
	for key, t := range s.values {
		if t == nil {
			continue
		}
		parsedSym, _, suffix, ok := pathkey.ParseKeyUnchecked(constraint.PathKey(key))
		if !ok || parsedSym != sym || suffix != "" {
			continue
		}
		types = append(types, t)
	}
	if len(types) == 0 {
		return nil
	}
	return join.Types(types...)
}

// resolveSymbolKeyType gets the flow-narrowed type for a symbol at point p.
//
// Used to look up key types for dynamic index operations. The symbol ID provides
// unique identity for SSA version lookup, and varName is used to construct
// the path for type lookup.
//
// Returns nil if symbol is zero, as identity-based lookup requires a valid Symbol.
func (s *Solution) resolveSymbolKeyType(p cfg.Point, sym cfg.SymbolID, varName string) typ.Type {
	if sym == 0 {
		return nil
	}
	keyPath := constraint.Path{Root: varName, Symbol: sym}
	if narrowed := s.NarrowedTypeAt(p, keyPath); narrowed != nil {
		return narrowed
	}
	return s.TypeAt(p, keyPath)
}

// processTableMutatorAssignmentReturnKey handles table.insert-like mutations.
//
// When table.insert(t, v) or similar operations are called, the array element
// type may need widening. This method:
//
//  1. Resolves value type from flow state or uses fallback
//  2. Skips annotated variables (explicit types are preserved)
//  3. For keyed targets (suites[name]), widens the map value's array element
//  4. For direct targets, widens the array element type
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processTableMutatorAssignmentReturnKey(p cfg.Point, tm TableMutatorAssignment) string {
	if tm.Target.Symbol == 0 {
		return ""
	}

	// Resolve value type from flow state or use fallback
	valueType := tm.ValueType
	if tm.ValuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, tm.ValuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if valueType == nil {
		return ""
	}

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[tm.Target.Symbol] {
			return ""
		}
	}

	pathKey := s.pkResolver.KeyAt(p, tm.Target)
	if pathKey == "" {
		return ""
	}

	currentType := s.values[string(pathKey)]
	currentType = preferDeclaredTemplateForWiden(currentType, s.lookupDeclaredType(constraint.Path{
		Root:   tm.Target.Root,
		Symbol: tm.Target.Symbol,
	}))

	var newType typ.Type
	if tm.KeySymbol != 0 || tm.KeyType != nil {
		keyType := tm.KeyType
		if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
			keyType = s.resolveSymbolKeyType(p, tm.KeySymbol, tm.KeyVar)
		}
		keyType = normalizeDynamicKeyType(keyType)
		newType = WidenMapValueArray(currentType, keyType, valueType)
	} else {
		newType = WidenArrayElementType(currentType, valueType, typ.JoinPreferNonSoft)
	}

	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.setValue(string(pathKey), newType)
	return string(pathKey)
}

func normalizeDynamicKeyType(keyType typ.Type) typ.Type {
	if keyType == nil || typ.IsAbsentOrUnknown(keyType) {
		return typ.Unknown
	}
	return subtype.Widen(keyType)
}

// processContainerMutatorAssignmentReturnKey handles container mutations.
//
// When container:send(v) or similar operations are called, the container's
// element type may need widening to include the sent value's type. This is
// controlled by ContainerElementUnion effects in function specs.
//
// The method:
//  1. Resolves value type from flow state (using ValuePath) or fallback
//  2. Applies WidenForInference to normalize the value type
//  3. Skips annotated variables (explicit types are preserved)
//  4. Widens the container's element type via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processContainerMutatorAssignmentReturnKey(p cfg.Point, cm ContainerMutatorAssignment) string {
	if cm.Target.Symbol == 0 {
		return ""
	}

	// Resolve value type from flow state or use fallback
	valueType := cm.ValueType
	if cm.ValuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, cm.ValuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if valueType == nil {
		return ""
	}
	valueType = subtype.WidenForInference(valueType)

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[cm.Target.Symbol] {
			return ""
		}
	}

	pathKey := s.pkResolver.KeyAt(p, cm.Target)
	if pathKey == "" {
		return ""
	}

	currentType := s.values[string(pathKey)]
	newType := widenContainerElementType(currentType, valueType)

	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.setValue(string(pathKey), newType)
	return string(pathKey)
}

// widenContainerElementType widens a container's element type by unioning with a new value type.
//
// Supports various container types:
//
//   - Instantiated generics (Channel<T>): Widens first type argument
//   - Arrays: Widens element type
//   - Maps: Widens value type (key unchanged)
//   - Unions: Recursively widens contained containers
//
// Special case for Unknown element type: Rather than unioning (which would keep Unknown),
// replaces Unknown entirely with the value type. This enables precise inference when
// a container starts with unknown element type and is populated with concrete values.
//
// Returns the widened type, or the original if no widening is applicable.
func widenContainerElementType(containerType typ.Type, valueType typ.Type) typ.Type {
	if valueType == nil {
		return containerType
	}
	if containerType == nil {
		return nil
	}

	return typ.Visit(containerType, typ.Visitor[typ.Type]{
		Instantiated: func(inst *typ.Instantiated) typ.Type {
			// Handle instantiated generic containers (e.g., Channel<T>)
			if inst.Generic == nil || len(inst.TypeArgs) == 0 {
				return containerType
			}
			// Get current element type
			oldElem := inst.TypeArgs[0]
			// If old element is unknown, replace it with value type (not union)
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			// Create new instantiation with widened element type
			newArgs := make([]typ.Type, len(inst.TypeArgs))
			copy(newArgs, inst.TypeArgs)
			newArgs[0] = newElem
			return typ.Instantiate(inst.Generic, newArgs...)
		},
		Array: func(arr *typ.Array) typ.Type {
			// Handle array types
			oldElem := arr.Element
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			return typ.NewArray(newElem)
		},
		Map: func(m *typ.Map) typ.Type {
			// Handle map types (widen value type)
			newVal := join.Types(m.Value, valueType)
			if typ.TypeEquals(m.Value, newVal) {
				return containerType
			}
			return typ.NewMap(m.Key, newVal)
		},
		Union: func(u *typ.Union) typ.Type {
			// Handle unions containing containers
			var updated []typ.Type
			changed := false
			for _, m := range u.Members {
				widened := widenContainerElementType(m, valueType)
				if widened != nil && !typ.TypeEquals(m, widened) {
					updated = append(updated, widened)
					changed = true
				} else {
					updated = append(updated, m)
				}
			}
			if changed {
				return typ.NewUnion(updated...)
			}
			return containerType
		},
		Default: func(t typ.Type) typ.Type {
			// For unknown/any, cannot widen
			return containerType
		},
	})
}

// WidenArrayElementType widens an array type by joining a new element type.
//
// This function handles the common pattern of widening arrays as elements are
// added. The join function controls how types are combined:
//
//   - For union: joinFn = join.Two (creates union types)
//   - For intersection: joinFn = narrow.Intersect (narrows types)
//
// Type handling:
//   - Array: Joins element type with new type
//   - Empty record: Converts to array with new element type
//   - Union: Widens first array member found
//   - nil/placeholder: Creates new array with element type
//   - Other types: Returns unchanged (not an array)
//
// Returns the widened type.
func WidenArrayElementType(arrayType typ.Type, elementType typ.Type, joinFn func(a, b typ.Type) typ.Type) typ.Type {
	if elementType == nil {
		return arrayType
	}

	if arrayType == nil {
		return typ.NewArray(elementType)
	}

	return typ.Visit(arrayType, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := WidenArrayElementType(a.Target, elementType, joinFn)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return arrayType
			}
			return typ.NewAlias(a.Name, widened)
		},
		Array: func(arr *typ.Array) typ.Type {
			return typ.NewArray(joinFn(arr.Element, elementType))
		},
		Record: func(rec *typ.Record) typ.Type {
			if len(rec.Fields) == 0 {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
		Union: func(u *typ.Union) typ.Type {
			var updated []typ.Type
			found := false
			for _, m := range u.Members {
				if arr, ok := m.(*typ.Array); ok && !found {
					updated = append(updated, typ.NewArray(joinFn(arr.Element, elementType)))
					found = true
				} else {
					updated = append(updated, m)
				}
			}
			if found {
				return typ.NewUnion(updated...)
			}
			return arrayType
		},
		Default: func(t typ.Type) typ.Type {
			if arrayType.Kind().IsPlaceholder() {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
	})
}

// WidenMapValueArray widens a map's value type by adding an element to its array component.
//
// This handles the pattern: suites[name] = suites[name] or {}; table.insert(suites[name], test)
// where suites is {[string]: Test[]} and we need to widen the array element type.
//
// Type handling:
//   - Map: Widens key type and array element of value type
//   - Empty record: Converts to map with array value
//   - Union: Widens first map member found
//   - Placeholder types: Creates new map with array value
//   - Other types: Returns unchanged
//
// Returns the widened type.
func WidenMapValueArray(mapType typ.Type, keyType, elementType typ.Type) typ.Type {
	if mapType == nil {
		return typ.NewMap(keyType, typ.NewArray(elementType))
	}

	return typ.Visit(mapType, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := WidenMapValueArray(a.Target, keyType, elementType)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return mapType
			}
			return typ.NewAlias(a.Name, widened)
		},
		Map: func(m *typ.Map) typ.Type {
			newKey := mergeMapKeyDomain(m.Key, keyType)
			newVal := WidenArrayElementType(m.Value, elementType, typ.JoinPreferNonSoft)
			if newVal == nil {
				return mapType
			}
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return mapType
			}
			return typ.NewMap(newKey, newVal)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) == 0 {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
		Union: func(u *typ.Union) typ.Type {
			var updated []typ.Type
			found := false
			for _, m := range u.Members {
				if mp, ok := m.(*typ.Map); ok && !found {
					newKey := mergeMapKeyDomain(mp.Key, keyType)
					newVal := WidenArrayElementType(mp.Value, elementType, typ.JoinPreferNonSoft)
					if newVal == nil {
						updated = append(updated, m)
					} else {
						updated = append(updated, typ.NewMap(newKey, newVal))
					}
					found = true
				} else {
					updated = append(updated, m)
				}
			}
			if found {
				return typ.NewUnion(updated...)
			}
			return mapType
		},
		Default: func(t typ.Type) typ.Type {
			if mapType.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
	})
}

func mergeMapKeyDomain(existing, incoming typ.Type) typ.Type {
	if existing == nil {
		return incoming
	}
	if incoming == nil {
		return existing
	}
	// Placeholder evidence (any/unknown) is non-informative for key domains.
	// Preserve an existing concrete domain instead of widening it.
	if incoming.Kind().IsPlaceholder() && !existing.Kind().IsPlaceholder() {
		return existing
	}
	if existing.Kind().IsPlaceholder() && !incoming.Kind().IsPlaceholder() {
		return incoming
	}
	return typ.JoinPreferNonSoft(existing, incoming)
}

func preferDeclaredTemplateForWiden(current, declared typ.Type) typ.Type {
	if declared == nil {
		return current
	}
	if current == nil || current.Kind().IsPlaceholder() || isEmptyRecordNoMapType(current) {
		return declared
	}
	return current
}

func isEmptyRecordNoMapType(t typ.Type) bool {
	switch v := t.(type) {
	case *typ.Alias:
		return isEmptyRecordNoMapType(v.Target)
	case *typ.Record:
		return len(v.Fields) == 0 && !v.HasMapComponent()
	default:
		return false
	}
}

// widenWithIndexer widens a type based on dynamic index assignment (t[k] = v).
//
// Dynamic indexing (non-constant keys) indicates the type needs a map component.
// This function transforms types to accommodate dynamic access:
//
//   - nil base: Creates map {[K]: V}
//   - Empty record {}: Converts to map {[K]: V}
//   - Record with fields: Adds or widens map component
//   - Existing map: Widens key/value types via union
//   - Placeholder types: Creates map {[K]: V}
//   - Other types: Returns unchanged
//
// Nil values are skipped: In Lua, t[k] = nil deletes the key rather than storing nil.
// Map access already returns Optional to represent potentially missing keys.
func widenWithIndexer(t typ.Type, keyType, valType typ.Type) typ.Type {
	if valType != nil && valType.Kind() == kind.Nil {
		return t
	}

	if t == nil {
		return typ.NewMap(keyType, valType)
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Alias: func(a *typ.Alias) typ.Type {
			widened := widenWithIndexer(a.Target, keyType, valType)
			if widened == nil || typ.TypeEquals(widened, a.Target) {
				return t
			}
			return typ.NewAlias(a.Name, widened)
		},
		Tuple: func(tp *typ.Tuple) typ.Type {
			elemType := valType
			for _, elem := range tp.Elements {
				elemType = typ.JoinPreferNonSoft(elemType, elem)
			}
			// Numeric tuple indexing indicates array-like mutation; widen to array.
			if subtype.IsSubtype(keyType, typ.Integer) || subtype.IsSubtype(keyType, typ.Number) {
				return typ.NewArray(elemType)
			}
			return typ.NewMap(keyType, elemType)
		},
		Record: func(r *typ.Record) typ.Type {
			// Empty record {} with no map component becomes a map (backward compat)
			if len(r.Fields) == 0 && !r.HasMapComponent() {
				return typ.NewMap(keyType, valType)
			}
			// Record with fields: add or widen map component
			if r.HasMapComponent() {
				newKey := mergeMapKeyDomain(r.MapKey, keyType)
				newVal := typ.JoinPreferNonSoft(r.MapValue, valType)
				if typ.TypeEquals(r.MapKey, newKey) && typ.TypeEquals(r.MapValue, newVal) {
					return t
				}
				return rebuildRecordWithMapComponent(r, newKey, newVal)
			}
			// Record with fields but no map component: add map component
			return rebuildRecordWithMapComponent(r, keyType, valType)
		},
		Map: func(m *typ.Map) typ.Type {
			// Widen existing map by unioning key/value types, preferring non-soft.
			newKey := mergeMapKeyDomain(m.Key, keyType)
			newVal := typ.JoinPreferNonSoft(m.Value, valType)
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return t
			}
			return typ.NewMap(newKey, newVal)
		},
		Default: func(t typ.Type) typ.Type {
			// For other types (unknown, any), create a map
			if t.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, valType)
			}
			return t
		},
	})
}

// rebuildRecordWithMapComponent creates a new record with an added or updated map component.
//
// Lua tables can have both named fields (record component) and dynamic keys
// (map component). This function preserves all fields, flags (optional, readonly),
// and metatable from the original record while adding or replacing the map component.
//
// The resulting type represents a table that can be accessed both by known field
// names and by dynamic keys of type mapKey.
func rebuildRecordWithMapComponent(rec *typ.Record, mapKey, mapVal typ.Type) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	builder.MapComponent(mapKey, mapVal)
	return builder.Build()
}

// processJoinReturnChangedKeys processes phi nodes at point p.
//
// Phi nodes occur at control flow join points where a variable may have different
// values from different predecessors. This method:
//
//  1. Collects types from each phi operand (applying edge conditions)
//  2. Joins all incoming types into a single result type
//  3. Stores the joined type under the phi target's canonical key
//  4. Processes field suffixes to propagate nested field narrowings through phis
//
// Field suffix processing enables patterns like:
//
//	if x.ok then x = {ok: true, v: 1} else x = {ok: false, e: ""} end
//	-- phi for x.ok merges true and false paths
//
// Returns keys that changed, driving worklist propagation.
func (s *Solution) processJoinReturnChangedKeys(p cfg.Point) []string {
	if s.inputs.Graph == nil {
		return nil
	}

	var changedKeys []string

	for _, phi := range s.inputs.Graph.PhiNodes() {
		if phi.Point != p {
			continue
		}

		// Collect types from operands, applying edge conditions
		types := s.scratchTypes[:0]
		for _, op := range phi.Operands {
			opType := s.phiOperandTypeAt(p, op, nil)
			if opType == nil {
				continue
			}
			types = append(types, opType)
		}

		if len(types) == 0 {
			continue
		}

		// Join and store under phi target canonical key
		joined := join.Types(types...)
		targetKey := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
		if targetKey == "" {
			continue
		}
		old := s.values[string(targetKey)]
		if !typ.TypeEquals(old, joined) {
			s.setValue(string(targetKey), joined)
			changedKeys = append(changedKeys, string(targetKey))
		}

		// Also process field suffixes from phi operands
		suffixMap := s.collectPhiOperandSuffixes(phi)
		suffixes := make([]string, 0, len(suffixMap))
		for suffix := range suffixMap {
			suffixes = append(suffixes, suffix)
		}
		sort.Strings(suffixes)
		for _, suffix := range suffixes {
			segments := pathkey.ParseSuffix(suffix)
			if len(segments) == 0 {
				continue
			}
			types = types[:0]
			for _, op := range phi.Operands {
				opType := s.phiOperandTypeAt(p, op, segments)
				if opType == nil {
					opType = typ.Nil
				}
				types = append(types, opType)
			}
			if len(types) == 0 {
				continue
			}
			joined = join.Types(types...)
			fullKey := string(targetKey) + suffix
			old = s.values[fullKey]
			if !typ.TypeEquals(old, joined) {
				s.setValue(fullKey, joined)
				changedKeys = append(changedKeys, fullKey)
			}
		}
	}

	return changedKeys
}

func (s *Solution) phiOperandTypeAt(joinPoint cfg.Point, op cfg.PhiOperand, segments []constraint.Segment) typ.Type {
	if s == nil {
		return nil
	}
	path := constraint.Path{
		Root:    op.Version.Root,
		Symbol:  op.Version.Symbol,
		Version: op.Version.ID,
	}
	if len(segments) > 0 {
		path.Segments = append(path.Segments, segments...)
	}

	opType := s.NarrowedTypeAt(op.From, path)
	if opType == nil {
		opType = s.baseTypeAt(op.From, path)
	}
	if opType == nil {
		return nil
	}

	edgeK := edgeKey{from: op.From, to: joinPoint}
	if cond, ok := s.edgeConditions[edgeK]; ok && cond.HasConstraints() {
		if narrowed := s.applyCondition(op.From, opType, path, cond); narrowed != nil {
			opType = narrowed
		}
	}
	return opType
}

// collectPhiOperandSuffixes collects field suffixes from phi operand canonical keys.
//
// When a phi merges versions of a variable, field assignments made on different
// branches also need to be joined. This method scans the values map to find all
// field/index suffixes that have types for any phi operand.
//
// For example, if phi merges x@1 and x@2, and values contains:
//   - x@1.foo -> string
//   - x@2.bar -> number
//
// This returns {".foo", ".bar"} so both field types can be joined at the phi.
//
// Uses scratch buffer s.scratchSuffix to reduce allocations in the hot path.
func (s *Solution) collectPhiOperandSuffixes(phi cfg.PhiNode) map[string]struct{} {
	out := s.scratchSuffix
	if out == nil {
		out = make(map[string]struct{}, 16)
		s.scratchSuffix = out
	}
	clear(out)

	for _, op := range phi.Operands {
		baseKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
		if baseKey == "" {
			continue
		}
		baseKeyStr := string(baseKey)
		baseLen := len(baseKeyStr)
		for key := range s.values {
			if len(key) <= baseLen {
				continue
			}
			// Check prefix match
			if key[:baseLen] != baseKeyStr {
				continue
			}
			// Extract suffix (must start with . or [)
			suffix := key[baseLen:]
			if len(suffix) > 0 && (suffix[0] == '.' || suffix[0] == '[') {
				out[suffix] = struct{}{}
			}
		}
	}
	return out
}
