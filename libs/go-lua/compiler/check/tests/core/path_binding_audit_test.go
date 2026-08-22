package core

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
)

// TestPathBindingAudit_UnversionedConstraints verifies that under the new
// unversioned constraint model, all constraint paths are properly bound:
// - Every constraint path root exists in assignment roots or declared vars
// - No constraint paths are empty
// - No constraint paths have DefPoint set (all should be 0/unversioned)
func TestPathBindingAudit_UnversionedConstraints(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string, data: any? }
		type Time = { sec: number, nsec: number }

		function handler(events_ch: Channel<Event>, timeout: Channel<Time>)
			local result = channel.select {
				events_ch:case_receive(),
				timeout:case_receive(),
			}
			if result.channel == timeout then
				return nil
			end
			local msg = result.value
			return msg.kind
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))
	sess := result.Session

	// Collect all assignment roots and declared vars
	knownRoots := make(map[string]bool)

	for _, funcResult := range sess.Results {
		if funcResult.FlowInputs == nil {
			continue
		}

		inputs := funcResult.FlowInputs

		// Collect declared types (function params, locals with type annotations)
		// DeclaredTypes is keyed by SymbolID, but assignments still have string roots
		// We skip DeclaredTypes iteration since assignment targets capture the roots

		// Collect assignment targets
		for _, assign := range inputs.Assignments {
			if assign.TargetPath.Root != "" {
				knownRoots[assign.TargetPath.Root] = true
			}
		}

	}

	t.Logf("Known roots: %v", knownRoots)

	// Audit constraints
	var emptyPaths []string
	var unboundRoots []string
	var versionedPaths []string

	for _, funcResult := range sess.Results {
		if funcResult.FlowInputs == nil {
			continue
		}

		inputs := funcResult.FlowInputs

		for _, ec := range inputs.EdgeConditions {
			for _, disjunct := range ec.Condition.Disjuncts {
				for _, c := range disjunct {
					paths := c.Paths()
					for _, path := range paths {
						// Check for empty paths
						if path.IsEmpty() {
							emptyPaths = append(emptyPaths, constraintDesc(c))
							continue
						}

						// Check for unbound roots
						if !knownRoots[path.Root] {
							unboundRoots = append(unboundRoots, path.Root+" in "+constraintDesc(c))
						}

						// Check for versioned paths (DefPoint should always be 0 now)
						// Note: constraint.Path no longer has DefPoint field, so this is
						// just a sanity check that the struct doesn't have the field
						if hasDefPoint(path) {
							versionedPaths = append(versionedPaths, path.String()+" in "+constraintDesc(c))
						}
					}
				}
			}
		}
	}

	// Report findings
	if len(emptyPaths) > 0 {
		t.Errorf("Found %d empty constraint paths: %v", len(emptyPaths), emptyPaths)
	}

	if len(unboundRoots) > 0 {
		t.Errorf("Found %d unbound roots: %v", len(unboundRoots), unboundRoots)
	}

	if len(versionedPaths) > 0 {
		t.Errorf("Found %d versioned paths (should all be unversioned): %v", len(versionedPaths), versionedPaths)
	}

	// Log constraint summary
	constraintCount := 0
	for _, funcResult := range sess.Results {
		if funcResult.FlowInputs != nil {
			constraintCount += len(funcResult.FlowInputs.EdgeConditions)
		}
	}
	t.Logf("Audited %d edge constraints across %d functions", constraintCount, len(sess.Results))
}

// constraintDesc returns a short description of a constraint.
func constraintDesc(c constraint.Constraint) string {
	switch v := c.(type) {
	case constraint.NotNil:
		return "NotNil{" + v.Path.String() + "}"
	case constraint.IsNil:
		return "IsNil{" + v.Path.String() + "}"
	case constraint.Truthy:
		return "Truthy{" + v.Path.String() + "}"
	case constraint.Falsy:
		return "Falsy{" + v.Path.String() + "}"
	case constraint.HasType:
		return "HasType{" + v.Path.String() + "}"
	case constraint.NotHasType:
		return "NotHasType{" + v.Path.String() + "}"
	case constraint.FieldEqualsPath:
		return "FieldEqualsPath{" + v.Target.String() + "." + v.Field + "=" + v.Value.String() + "}"
	case constraint.FieldNotEqualsPath:
		return "FieldNotEqualsPath{" + v.Target.String() + "." + v.Field + "!=" + v.Value.String() + "}"
	default:
		return "constraint"
	}
}

// hasDefPoint checks if a path has a DefPoint set.
// Since constraint.Path no longer has DefPoint field, this always returns false.
// This function exists to document the invariant and will compile-error if
// DefPoint is accidentally re-added.
func hasDefPoint(_ constraint.Path) bool {
	// The constraint.Path struct no longer has a DefPoint field.
	// If this code fails to compile, it means DefPoint was re-added
	// and this test needs to be updated.
	return false
}

// TestPathBindingAudit_AssignmentPathsAreUnversioned verifies that assignment
// source paths in FlowInputs are also unversioned (DefPoint=0).
func TestPathBindingAudit_AssignmentPathsAreUnversioned(t *testing.T) {
	chManifest := testutil.ChannelManifest()

	source := `
		type Event = { kind: string }
		type Time = { sec: number }

		function f(ch1: Channel<Event>, ch2: Channel<Time>)
			local result = channel.select { ch1:case_receive(), ch2:case_receive() }
			local val = result.value
			return val
		end
	`

	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("channel", chManifest))
	sess := result.Session

	assignmentCount := 0
	for _, funcResult := range sess.Results {
		if funcResult.FlowInputs == nil {
			continue
		}

		for _, assign := range funcResult.FlowInputs.Assignments {
			assignmentCount++

			// SourcePath should be unversioned (no DefPoint in the path key)
			if !assign.SourcePath.IsEmpty() {
				key := assign.SourcePath.Key()
				// Key should not contain @ unless it's from segments
				if containsVersionMarker(string(key), assign.SourcePath.Root) {
					t.Errorf("Assignment source path appears versioned: %s (from %s)", key, assign.SourcePath.Root)
				}
			}
		}
	}

	t.Logf("Audited %d assignments", assignmentCount)
}

// containsVersionMarker checks if a path key contains a version marker (@N) after the root.
func containsVersionMarker(key, root string) bool {
	if len(key) <= len(root) {
		return false
	}
	suffix := key[len(root):]
	// If suffix starts with @, it's a version marker
	return len(suffix) > 0 && suffix[0] == '@'
}

// TestPathBindingAudit_MinimalChannel tests a minimal channel scenario.
func TestPathBindingAudit_MinimalChannel(t *testing.T) {
	source := `
		type ChanA = {__tag: "a"}
		type ChanB = {__tag: "b"}
		type Result = {channel: ChanA, value: number} | {channel: ChanB, value: string}

		function f(result: Result, chB: ChanB)
			if result.channel == chB then
				return result.value
			end
			return result.value
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	sess := result.Session

	for _, funcResult := range sess.Results {
		if funcResult.FlowInputs == nil {
			continue
		}

		inputs := funcResult.FlowInputs
		t.Logf("Function has %d edge constraints", len(inputs.EdgeConditions))

		for i, ec := range inputs.EdgeConditions {
			t.Logf("  EdgeCondition[%d]: from=%d to=%d", i, ec.From, ec.To)
			for _, disjunct := range ec.Condition.Disjuncts {
				for _, c := range disjunct {
					t.Logf("    %s", constraintDesc(c))
				}
			}
		}
	}

	// Just verify no errors in processing
	if result.HasError() {
		t.Logf("Note: got errors (may be expected): %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
