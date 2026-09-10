package constraint

// VisitPaths calls fn for each path referenced by c.
//
// It is the allocation-free counterpart to Constraint.Paths(). Callers must
// treat visited paths as read-only.
func VisitPaths(c Constraint, fn func(Path) bool) bool {
	switch v := c.(type) {
	case Truthy:
		if fn(v.Path) {
			return true
		}
		return visitParentFieldPath(v.Path, fn)
	case Falsy:
		if fn(v.Path) {
			return true
		}
		return visitParentFieldPath(v.Path, fn)
	case IsNil:
		return fn(v.Path)
	case NotNil:
		return fn(v.Path)
	case HasType:
		return fn(v.Path)
	case NotHasType:
		return fn(v.Path)
	case HasField:
		return fn(v.Path)
	case FieldEquals:
		if fn(v.Target) {
			return true
		}
		return visitParentFieldPath(v.Target, fn)
	case FieldNotEquals:
		if fn(v.Target) {
			return true
		}
		return visitParentFieldPath(v.Target, fn)
	case IndexEquals:
		return fn(v.Target)
	case IndexNotEquals:
		return fn(v.Target)
	case EqPath:
		return fn(v.Left) || fn(v.Right)
	case NotEqPath:
		return fn(v.Left) || fn(v.Right)
	case FieldEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case FieldNotEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case IndexEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case IndexNotEqualsPath:
		return fn(v.Target) || fn(v.Value)
	case KeyOf:
		return fn(v.Table) || fn(v.Key)
	default:
		return false
	}
}

// FirstPath returns the first path referenced by c, if any.
func FirstPath(c Constraint) (Path, bool) {
	var first Path
	ok := false
	VisitPaths(c, func(p Path) bool {
		first = p
		ok = true
		return true
	})
	return first, ok
}

func visitParentFieldPath(path Path, fn func(Path) bool) bool {
	if len(path.Segments) == 0 {
		return false
	}
	if path.Segments[len(path.Segments)-1].Kind != SegmentField {
		return false
	}
	parent := Path{Root: path.Root, Symbol: path.Symbol}
	if len(path.Segments) > 1 {
		parent.Segments = path.Segments[:len(path.Segments)-1]
	}
	return fn(parent)
}
