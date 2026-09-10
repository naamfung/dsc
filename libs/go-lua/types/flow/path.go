package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/pathkey"
)

// parsePath parses a path string into a constraint.Path for display purposes.
//
// This function parses path syntax like "x", "x.field", "x[0]", or "x[\"key\"]"
// into a structured Path representation. The parsing supports:
//
//   - Simple identifiers: "x" becomes Path{Root: "x"}
//   - Field access: "x.foo" becomes Path{Root: "x", Segments: [.foo]}
//   - Integer index: "x[0]" becomes Path{Root: "x", Segments: [[0]]}
//   - String index: "x[\"key\"]" becomes Path{Root: "x", Segments: [["key"]]}
//
// The returned Path has Symbol=0 and MUST NOT be used for value identity in
// the flow solver. For identity-aware path resolution that binds to SSA versions,
// use the pathkey.Resolver or binding-aware extractors in flowbuild/path.
//
// Returns (Path, true) on successful parse, or (empty Path, false) on error.
// The returned Path has Symbol=0 and MUST NOT be used for value identity in the solver.
// For identity-aware paths, use PathFromExprFull with a SymbolResolver.
func parsePath(path string) (constraint.Path, bool) {
	if path == "" {
		return constraint.Path{}, false
	}

	i := 0

	root := pathkey.ReadIdent(path, &i)
	if root == "" {
		return constraint.Path{}, false
	}

	suffix := path[i:]
	segs := pathkey.ParseSuffix(suffix)
	if suffix != "" && len(segs) == 0 {
		return constraint.Path{}, false
	}

	return constraint.Path{Root: root, Segments: segs}, true
}
