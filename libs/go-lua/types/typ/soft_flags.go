package typ

// softPruneMayRewrite reports whether PruneSoftUnionMembers can rewrite t or
// any of its descendants. The flag is computed once at type construction time
// for immutable structural types so hot paths can skip recursive descent.
func softPruneMayRewrite(t Type) bool {
	if t == nil {
		return false
	}
	switch node := unwrapTransparentSoft(t).(type) {
	case *Union:
		return node.softPrunable
	case *Optional:
		return node.softPrunable
	case *Array:
		return node.softPrunable
	case *Map:
		return node.softPrunable
	case *Tuple:
		return node.softPrunable
	case *Function:
		return node.softPrunable
	case *Record:
		return node.softPrunable
	case *Alias:
		return node.softPrunable
	case *Instantiated:
		return node.softPrunable
	default:
		return false
	}
}

func softPruneAny(types ...Type) bool {
	for _, t := range types {
		if softPruneMayRewrite(t) {
			return true
		}
	}
	return false
}

func softPruneParams(params []Param) bool {
	for _, p := range params {
		if softPruneMayRewrite(p.Type) {
			return true
		}
	}
	return false
}

func softPruneFields(fields []Field) bool {
	for _, f := range fields {
		if softPruneMayRewrite(f.Type) {
			return true
		}
	}
	return false
}
