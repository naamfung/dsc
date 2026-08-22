package analysis

import (
	"testing"

	basecfg "github.com/wippyai/go-lua/types/cfg"
)

// buildStraightLine creates a simple linear CFG:
// entry -> n1 -> n2 -> n3 -> exit.
func buildStraightLine() basecfg.Graph {
	cfg := basecfg.New()
	n1 := cfg.AddNode(basecfg.NodeAssign, 0, "")
	n2 := cfg.AddNode(basecfg.NodeAssign, 0, "")
	n3 := cfg.AddNode(basecfg.NodeAssign, 0, "")

	cfg.AddEdge(cfg.Entry(), n1, false)
	cfg.AddEdge(n1, n2, false)
	cfg.AddEdge(n2, n3, false)
	cfg.AddEdge(n3, cfg.Exit(), false)

	return cfg
}

// buildIfElse creates a diamond CFG:
//
//	   entry
//	     |
//	   branch
//	   /    \
//	then   else
//	   \    /
//	    join
//	     |
//	    exit
func buildIfElse() basecfg.Graph {
	cfg := basecfg.New()
	branch := cfg.AddNode(basecfg.NodeBranch, 0, "")
	thenN := cfg.AddNode(basecfg.NodeAssign, 0, "")
	elseN := cfg.AddNode(basecfg.NodeAssign, 0, "")
	join := cfg.AddNode(basecfg.NodeJoin, 0, "")

	cfg.AddEdge(cfg.Entry(), branch, false)
	cfg.AddEdge(branch, thenN, true)
	cfg.AddEdge(branch, elseN, false)
	cfg.AddEdge(thenN, join, false)
	cfg.AddEdge(elseN, join, false)
	cfg.AddEdge(join, cfg.Exit(), false)

	return cfg
}

// buildSimpleLoop creates a loop CFG:
//
//	entry
//	  |
//	header <---+
//	  |        |
//	body ------+
//	  |
//	exit
func buildSimpleLoop() basecfg.Graph {
	cfg := basecfg.New()
	header := cfg.AddNode(basecfg.NodeBranch, 0, "")
	body := cfg.AddNode(basecfg.NodeAssign, 0, "")

	cfg.AddEdge(cfg.Entry(), header, false)
	cfg.AddEdge(header, body, true)
	cfg.AddEdge(header, cfg.Exit(), false)
	cfg.AddEdge(body, header, false)

	return cfg
}

// buildNestedIf creates a nested if structure:
//
//	    entry
//	      |
//	   branch1
//	   /     \
//	branch2   n3
//	/    \     |
//	n1   n2    |
//	\   /      |
//	join1     /
//	    \    /
//	    join2
//	      |
//	    exit
func buildNestedIf() basecfg.Graph {
	cfg := basecfg.New()
	branch1 := cfg.AddNode(basecfg.NodeBranch, 0, "")
	branch2 := cfg.AddNode(basecfg.NodeBranch, 0, "")
	n1 := cfg.AddNode(basecfg.NodeAssign, 0, "")
	n2 := cfg.AddNode(basecfg.NodeAssign, 0, "")
	n3 := cfg.AddNode(basecfg.NodeAssign, 0, "")
	join1 := cfg.AddNode(basecfg.NodeJoin, 0, "")
	join2 := cfg.AddNode(basecfg.NodeJoin, 0, "")

	cfg.AddEdge(cfg.Entry(), branch1, false)
	cfg.AddEdge(branch1, branch2, true)
	cfg.AddEdge(branch1, n3, false)
	cfg.AddEdge(branch2, n1, true)
	cfg.AddEdge(branch2, n2, false)
	cfg.AddEdge(n1, join1, false)
	cfg.AddEdge(n2, join1, false)
	cfg.AddEdge(join1, join2, false)
	cfg.AddEdge(n3, join2, false)
	cfg.AddEdge(join2, cfg.Exit(), false)

	return cfg
}

func TestComputeDominators_StraightLine(t *testing.T) {
	g := buildStraightLine()
	idom, domTree := ComputeDominators(g)

	rpo := g.RPO()
	if len(rpo) != 5 {
		t.Fatalf("expected 5 nodes in RPO, got %d", len(rpo))
	}

	entry := g.Entry()
	exit := g.Exit()

	// Entry dominates itself
	if idom[entry] != entry {
		t.Errorf("entry idom should be entry, got %d", idom[entry])
	}

	// Each node except entry should have predecessor as idom
	for i := 1; i < len(rpo); i++ {
		p := rpo[i]
		expected := rpo[i-1]

		if idom[p] != expected {
			t.Errorf("idom[%d] = %d, want %d", p, idom[p], expected)
		}
	}

	// Dominator tree: entry has 1 child (n1)
	if len(domTree[entry]) != 1 {
		t.Errorf("entry should have 1 child, got %d", len(domTree[entry]))
	}

	// Exit should have no children
	if len(domTree[exit]) != 0 {
		t.Errorf("exit should have 0 children, got %d", len(domTree[exit]))
	}
}

func TestComputeDominators_IfElse(t *testing.T) {
	g := buildIfElse()
	idom, _ := ComputeDominators(g)

	rpo := g.RPO()
	entry := g.Entry()

	// Find specific nodes
	var branch, thenN, elseN, join basecfg.Point
	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			branch = p
		case basecfg.NodeJoin:
			join = p
		case basecfg.NodeAssign:
			if thenN == 0 {
				thenN = p
			} else {
				elseN = p
			}
		}
	}

	// Entry dominates itself
	if idom[entry] != entry {
		t.Errorf("entry idom should be entry")
	}

	// Branch is dominated by entry
	if idom[branch] != entry {
		t.Errorf("branch idom should be entry, got %d", idom[branch])
	}

	// Then and else are dominated by branch
	if idom[thenN] != branch {
		t.Errorf("then idom should be branch, got %d", idom[thenN])
	}
	if idom[elseN] != branch {
		t.Errorf("else idom should be branch, got %d", idom[elseN])
	}

	// Join is dominated by branch (not then or else)
	if idom[join] != branch {
		t.Errorf("join idom should be branch, got %d", idom[join])
	}
}

func TestComputeDominanceFrontier_IfElse(t *testing.T) {
	g := buildIfElse()
	idom, _ := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)

	rpo := g.RPO()

	// Find specific nodes
	var thenN, elseN, join basecfg.Point
	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeJoin:
			join = p
		case basecfg.NodeAssign:
			if thenN == 0 {
				thenN = p
			} else {
				elseN = p
			}
		}
	}

	// Then and else should have join in their DF
	if len(df[thenN]) != 1 || df[thenN][0] != join {
		t.Errorf("DF[then] should be {join}, got %v", df[thenN])
	}
	if len(df[elseN]) != 1 || df[elseN][0] != join {
		t.Errorf("DF[else] should be {join}, got %v", df[elseN])
	}

	// Entry and branch should have empty DF
	if len(df[g.Entry()]) != 0 {
		t.Errorf("DF[entry] should be empty, got %v", df[g.Entry()])
	}
}

func TestComputeDominators_SimpleLoop(t *testing.T) {
	g := buildSimpleLoop()
	idom, _ := ComputeDominators(g)

	entry := g.Entry()
	rpo := g.RPO()

	// Find header and body
	var header, body basecfg.Point

	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			header = p
		case basecfg.NodeAssign:
			body = p
		}
	}

	// Header is dominated by entry
	if idom[header] != entry {
		t.Errorf("header idom should be entry, got %d", idom[header])
	}

	// Body is dominated by header
	if idom[body] != header {
		t.Errorf("body idom should be header, got %d", idom[body])
	}

	// Exit is dominated by header
	if idom[g.Exit()] != header {
		t.Errorf("exit idom should be header, got %d", idom[g.Exit()])
	}
}

func TestComputeDominanceFrontier_SimpleLoop(t *testing.T) {
	g := buildSimpleLoop()
	idom, _ := ComputeDominators(g)
	df := ComputeDominanceFrontier(g, idom)

	rpo := g.RPO()

	// Find header and body
	var header, body basecfg.Point

	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			header = p
		case basecfg.NodeAssign:
			body = p
		}
	}

	// Body's DF should include header (loop back edge)
	if len(df[body]) != 1 || df[body][0] != header {
		t.Errorf("DF[body] should be {header}, got %v", df[body])
	}

	// Header is in its own DF because body (dominated by header) is a predecessor
	// of header, but header doesn't strictly dominate itself
	if len(df[header]) != 1 || df[header][0] != header {
		t.Errorf("DF[header] should be {header} (self-loop), got %v", df[header])
	}
}

func TestComputeDominators_NestedIf(t *testing.T) {
	g := buildNestedIf()
	idom, _ := ComputeDominators(g)

	entry := g.Entry()
	rpo := g.RPO()

	// Find nodes
	var (
		branch1, branch2 basecfg.Point
		joins            []basecfg.Point
	)

	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			if branch1 == 0 {
				branch1 = p
			} else {
				branch2 = p
			}
		case basecfg.NodeJoin:
			joins = append(joins, p)
		}
	}

	if len(joins) != 2 {
		t.Fatalf("expected 2 joins, got %d", len(joins))
	}

	join1 := joins[0]
	join2 := joins[1]

	// Branch1 dominated by entry
	if idom[branch1] != entry {
		t.Errorf("branch1 idom should be entry, got %d", idom[branch1])
	}

	// Branch2 dominated by branch1
	if idom[branch2] != branch1 {
		t.Errorf("branch2 idom should be branch1, got %d", idom[branch2])
	}

	// Join1 dominated by branch2
	if idom[join1] != branch2 {
		t.Errorf("join1 idom should be branch2, got %d", idom[join1])
	}

	// Join2 dominated by branch1
	if idom[join2] != branch1 {
		t.Errorf("join2 idom should be branch1, got %d", idom[join2])
	}
}

func TestDeterminism(t *testing.T) {
	// Run multiple times and verify same results
	for run := range 5 {
		g := buildNestedIf()
		idom1, domTree1 := ComputeDominators(g)
		df1 := ComputeDominanceFrontier(g, idom1)

		idom2, domTree2 := ComputeDominators(g)
		df2 := ComputeDominanceFrontier(g, idom2)

		// Compare idom
		for p, d := range idom1 {
			if idom2[p] != d {
				t.Errorf("run %d: idom mismatch at %d: %d vs %d", run, p, d, idom2[p])
			}
		}

		// Compare domTree order
		for p, children1 := range domTree1 {
			children2 := domTree2[p]

			if len(children1) != len(children2) {
				t.Errorf("run %d: domTree mismatch at %d: different lengths", run, p)

				continue
			}

			for i := range children1 {
				if children1[i] != children2[i] {
					t.Errorf("run %d: domTree order mismatch at %d[%d]: %d vs %d",
						run, p, i, children1[i], children2[i])
				}
			}
		}

		// Compare DF order
		for p, df1p := range df1 {
			df2p := df2[p]
			if len(df1p) != len(df2p) {
				t.Errorf("run %d: DF mismatch at %d: different lengths", run, p)

				continue
			}

			for i := range df1p {
				if df1p[i] != df2p[i] {
					t.Errorf("run %d: DF order mismatch at %d[%d]: %d vs %d",
						run, p, i, df1p[i], df2p[i])
				}
			}
		}
	}
}

func TestDominates(t *testing.T) {
	g := buildIfElse()
	idom, _ := ComputeDominators(g)

	entry := g.Entry()
	exit := g.Exit()
	rpo := g.RPO()

	var branch, join basecfg.Point

	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			branch = p
		case basecfg.NodeJoin:
			join = p
		}
	}

	// Entry dominates all
	for _, p := range rpo {
		if !Dominates(idom, entry, p) {
			t.Errorf("entry should dominate %d", p)
		}
	}

	// Branch dominates join
	if !Dominates(idom, branch, join) {
		t.Error("branch should dominate join")
	}

	// Join does not dominate branch
	if Dominates(idom, join, branch) {
		t.Error("join should not dominate branch")
	}

	// Exit does not dominate entry
	if Dominates(idom, exit, entry) {
		t.Error("exit should not dominate entry")
	}
}

func TestComputePostDominators_Linear(t *testing.T) {
	g := buildStraightLine()
	postIdom, _ := ComputePostDominators(g)

	rpo := g.RPO()
	exit := g.Exit()

	// Exit post-dominates all nodes in a linear chain
	for _, p := range rpo {
		if !PostDominates(postIdom, exit, p) {
			t.Errorf("exit should post-dominate %d", p)
		}
	}

	// Each node is post-dominated by its successor
	for i := range len(rpo) - 1 {
		if !PostDominates(postIdom, rpo[i+1], rpo[i]) {
			t.Errorf("%d should post-dominate %d", rpo[i+1], rpo[i])
		}
	}
}

func TestComputePostDominators_Diamond(t *testing.T) {
	g := buildIfElse()
	postIdom, _ := ComputePostDominators(g)

	rpo := g.RPO()
	exit := g.Exit()

	var branch, thenN, elseN, join basecfg.Point
	for _, p := range rpo {
		node := g.Node(p)
		switch node.Kind {
		case basecfg.NodeBranch:
			branch = p
		case basecfg.NodeJoin:
			join = p
		case basecfg.NodeAssign:
			if thenN == 0 {
				thenN = p
			} else {
				elseN = p
			}
		}
	}

	// Join post-dominates both then and else branches
	if !PostDominates(postIdom, join, thenN) {
		t.Error("join should post-dominate then")
	}

	if !PostDominates(postIdom, join, elseN) {
		t.Error("join should post-dominate else")
	}

	// Join post-dominates branch
	if !PostDominates(postIdom, join, branch) {
		t.Error("join should post-dominate branch")
	}

	// Exit post-dominates all
	for _, p := range rpo {
		if !PostDominates(postIdom, exit, p) {
			t.Errorf("exit should post-dominate %d", p)
		}
	}

	// Branch does NOT post-dominate join
	if PostDominates(postIdom, branch, join) {
		t.Error("branch should not post-dominate join")
	}
}

func TestPostDominates(t *testing.T) {
	g := buildIfElse()
	postIdom, _ := ComputePostDominators(g)

	entry := g.Entry()
	exit := g.Exit()
	rpo := g.RPO()

	var join basecfg.Point

	for _, p := range rpo {
		node := g.Node(p)
		if node.Kind == basecfg.NodeJoin {
			join = p
		}
	}

	// Exit post-dominates entry
	if !PostDominates(postIdom, exit, entry) {
		t.Error("exit should post-dominate entry")
	}

	// Entry does NOT post-dominate exit
	if PostDominates(postIdom, entry, exit) {
		t.Error("entry should not post-dominate exit")
	}

	// Self post-dominance
	if !PostDominates(postIdom, join, join) {
		t.Error("join should post-dominate itself")
	}
}

func TestStrictlyDominates(t *testing.T) {
	g := buildStraightLine()
	idom, _ := ComputeDominators(g)

	entry := g.Entry()

	// Entry strictly dominates all except itself
	for _, p := range g.RPO() {
		if p == entry {
			if StrictlyDominates(idom, entry, p) {
				t.Error("entry should not strictly dominate itself")
			}
		} else {
			if !StrictlyDominates(idom, entry, p) {
				t.Errorf("entry should strictly dominate %d", p)
			}
		}
	}
}

func TestComputeDomInfo(t *testing.T) {
	g := buildIfElse()
	info := ComputeDomInfo(g)

	if info == nil {
		t.Fatal("DomInfo should not be nil")
	}

	if info.ImmediateDominators == nil {
		t.Error("ImmediateDominators should not be nil")
	}

	if info.DominatorTree == nil {
		t.Error("DominatorTree should not be nil")
	}

	if info.DominanceFrontier == nil {
		t.Error("DominanceFrontier should not be nil")
	}
}

func TestEmptyGraph(t *testing.T) {
	cfg := basecfg.New()
	// No edges added, only entry and exit exist

	idom, domTree := ComputeDominators(cfg)
	df := ComputeDominanceFrontier(cfg, idom)

	// Entry dominates itself
	if idom[cfg.Entry()] != cfg.Entry() {
		t.Errorf("entry idom should be entry")
	}

	// Should not panic, results should be empty or minimal
	_ = domTree
	_ = df
}
