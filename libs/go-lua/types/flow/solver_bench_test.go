package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/typ"
)

func buildLargeCFG(branches, depth int) *cfg.CFG {
	c := cfg.New()
	current := c.Entry()

	for d := 0; d < depth; d++ {
		branch := c.AddNode(cfg.NodeBranch, cfg.SymbolID(0), "")
		c.AddEdge(current, branch, true)

		var joins []cfg.Point
		for b := 0; b < branches; b++ {
			assign := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
			c.AddEdge(branch, assign, b == 0)
			joins = append(joins, assign)
		}

		join := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
		for _, j := range joins {
			c.AddEdge(j, join, true)
		}
		current = join
	}

	c.AddEdge(current, c.Exit(), true)
	return c
}

func buildLoopyCFG(loopCount, bodySize int) *cfg.CFG {
	c := cfg.New()
	current := c.Entry()

	for i := 0; i < loopCount; i++ {
		header := c.AddNode(cfg.NodeJoin, cfg.SymbolID(0), "")
		c.AddEdge(current, header, true)

		prev := header
		for j := 0; j < bodySize; j++ {
			body := c.AddNode(cfg.NodeAssign, cfg.SymbolID(0), "")
			c.AddEdge(prev, body, true)
			prev = body
		}
		c.AddEdge(prev, header, true) // back-edge
		current = header
	}

	c.AddEdge(current, c.Exit(), false)
	return c
}

func BenchmarkSolve_SmallBranch(b *testing.B) {
	c, branch, thenNode := buildBranchCFG()
	g := newMockSSAGraph(c)

	allPoints := []cfg.Point{c.Entry(), branch, thenNode, c.Exit()}
	symX := setupSymbol(g, "x", allPoints)
	verX := cfg.Version{Root: "x", Symbol: symX, ID: 1}
	for _, p := range allPoints {
		setVersion(g, p, symX, verX)
	}

	inputs := newInputs(g)
	inputs.DeclaredTypes[symX] = typ.NewUnion(typ.String, typ.Number)

	pathX := constraint.Path{Root: "x"}
	inputs.EdgeConditions = []EdgeCondition{
		{
			From:      branch,
			To:        thenNode,
			Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}

func BenchmarkSolve_ManyBranches(b *testing.B) {
	c := buildLargeCFG(4, 10)
	g := newMockSSAGraph(c)
	inputs := newInputs(g)

	allPoints := c.RPO()
	syms := make(map[string]cfg.SymbolID)
	for i := 0; i < 20; i++ {
		name := string(rune('a' + i))
		sym := setupSymbol(g, name, allPoints)
		ver := cfg.Version{Root: name, Symbol: sym, ID: 1}
		for _, p := range allPoints {
			setVersion(g, p, sym, ver)
		}
		syms[name] = sym
		inputs.DeclaredTypes[sym] = typ.NewUnion(typ.String, typ.Number, typ.Boolean)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}

func BenchmarkSolve_DeepNesting(b *testing.B) {
	c := buildLargeCFG(2, 50)
	g := newMockSSAGraph(c)
	inputs := newInputs(g)

	allPoints := c.RPO()
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		sym := setupSymbol(g, name, allPoints)
		ver := cfg.Version{Root: name, Symbol: sym, ID: 1}
		for _, p := range allPoints {
			setVersion(g, p, sym, ver)
		}
		inputs.DeclaredTypes[sym] = typ.NewUnion(typ.String, typ.Number)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}

func BenchmarkSolve_LoopConvergence(b *testing.B) {
	c := buildLoopyCFG(5, 10)
	g := newMockSSAGraph(c)
	inputs := newInputs(g)

	allPoints := c.RPO()
	syms := make(map[string]cfg.SymbolID)
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		sym := setupSymbol(g, name, allPoints)
		ver := cfg.Version{Root: name, Symbol: sym, ID: 1}
		for _, p := range allPoints {
			setVersion(g, p, sym, ver)
		}
		syms[name] = sym
		inputs.DeclaredTypes[sym] = typ.String
	}

	rpo := c.RPO()
	for i, p := range rpo {
		if i%5 == 0 && i > 0 {
			name := string(rune('a' + (i % 10)))
			inputs.Assignments = append(inputs.Assignments, UnifiedAssignment{
				Point: p,
				TargetPath: constraint.Path{
					Root:   name,
					Symbol: syms[name],
				},
				Type: typ.Number,
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}

func BenchmarkSolve_ManyConstraints(b *testing.B) {
	c := buildLargeCFG(2, 20)
	g := newMockSSAGraph(c)
	inputs := newInputs(g)

	allPoints := c.RPO()
	syms := make(map[string]cfg.SymbolID)
	for i := 0; i < 10; i++ {
		name := string(rune('a' + i))
		sym := setupSymbol(g, name, allPoints)
		ver := cfg.Version{Root: name, Symbol: sym, ID: 1}
		for _, p := range allPoints {
			setVersion(g, p, sym, ver)
		}
		syms[name] = sym
		inputs.DeclaredTypes[sym] = typ.NewUnion(typ.String, typ.Number, typ.Boolean, typ.Integer)
	}

	rpo := c.RPO()
	for i := 1; i < len(rpo)-1; i++ {
		from := rpo[i]
		succs := c.Successors(from)
		if len(succs) > 0 {
			to := succs[0]
			name := string(rune('a' + (i % 10)))
			pathX := constraint.Path{Root: name}
			inputs.EdgeConditions = append(inputs.EdgeConditions, EdgeCondition{
				From:      from,
				To:        to,
				Condition: constraint.FromConstraints(constraint.HasType{Path: pathX, Type: narrow.BuiltinTypeKey("string")}),
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}

func BenchmarkSolve_NumericConstraints(b *testing.B) {
	c := buildLargeCFG(2, 15)
	g := newMockSSAGraph(c)
	inputs := newInputs(g)

	allPoints := c.RPO()
	syms := make(map[string]cfg.SymbolID)
	for i := 0; i < 5; i++ {
		name := string(rune('a' + i))
		sym := setupSymbol(g, name, allPoints)
		ver := cfg.Version{Root: name, Symbol: sym, ID: 1}
		for _, p := range allPoints {
			setVersion(g, p, sym, ver)
		}
		syms[name] = sym
		inputs.DeclaredTypes[sym] = typ.Integer
	}

	rpo := c.RPO()
	for i := 1; i < len(rpo)-1; i++ {
		from := rpo[i]
		succs := c.Successors(from)
		if len(succs) > 0 {
			to := succs[0]
			name := string(rune('a' + (i % 5)))
			pathX := constraint.Path{Root: name}
			inputs.EdgeNumericConstraints = append(inputs.EdgeNumericConstraints, EdgeNumericConstraint{
				From: from,
				To:   to,
				Constraints: []constraint.NumericConstraint{
					constraint.GeConst{X: pathX, C: int64(i)},
					constraint.LeConst{X: pathX, C: int64(i + 100)},
				},
			})
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Solve(inputs, testResolver())
	}
}
