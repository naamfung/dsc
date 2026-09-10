package constraint

import (
	"strconv"
	"testing"
)

func TestParamPath(t *testing.T) {
	tests := []struct {
		index    int
		wantRoot string
	}{
		{0, "$0"},
		{1, "$1"},
		{2, "$2"},
		{10, "$10"},
		{99, "$99"},
	}
	for _, tt := range tests {
		p := ParamPath(tt.index)
		if p.Root != tt.wantRoot {
			t.Errorf("ParamPath(%d).Root = %q, want %q", tt.index, p.Root, tt.wantRoot)
		}
		if p.Symbol != 0 {
			t.Errorf("ParamPath(%d).Symbol = %d, want 0", tt.index, p.Symbol)
		}
		if len(p.Segments) != 0 {
			t.Errorf("ParamPath(%d) should have no segments", tt.index)
		}
	}
}

func TestParamPath_IsPlaceholder(t *testing.T) {
	for i := 0; i < 10; i++ {
		p := ParamPath(i)
		if !p.IsPlaceholder() {
			t.Errorf("ParamPath(%d) should be a placeholder", i)
		}
		idx := p.PlaceholderIndex()
		if idx < 0 {
			t.Errorf("ParamPath(%d).PlaceholderIndex() should return non-negative, got %d", i, idx)
		}
		if idx != i {
			t.Errorf("ParamPath(%d).PlaceholderIndex() = %d, want %d", i, idx, i)
		}
	}
}

func TestRetPath(t *testing.T) {
	tests := []struct {
		index    int
		wantRoot string
	}{
		{0, "ret[0]"},
		{1, "ret[1]"},
		{2, "ret[2]"},
		{10, "ret[10]"},
	}
	for _, tt := range tests {
		p := RetPath(tt.index)
		if p.Root != tt.wantRoot {
			t.Errorf("RetPath(%d).Root = %q, want %q", tt.index, p.Root, tt.wantRoot)
		}
		if p.Symbol != 0 {
			t.Errorf("RetPath(%d).Symbol = %d, want 0", tt.index, p.Symbol)
		}
	}
}

func TestRetPath_NotPlaceholder(t *testing.T) {
	for i := 0; i < 5; i++ {
		p := RetPath(i)
		if p.IsPlaceholder() {
			t.Errorf("RetPath(%d) should not be a placeholder", i)
		}
	}
}

func TestPathHelpers_PathKey(t *testing.T) {
	// ParamPath should generate valid path keys
	p := ParamPath(0)
	key := p.Key()
	if key == "" {
		t.Error("ParamPath(0).Key() should not be empty")
	}

	// RetPath should generate valid path keys
	r := RetPath(0)
	rkey := r.Key()
	if rkey == "" {
		t.Error("RetPath(0).Key() should not be empty")
	}

	// Different paths should have different keys
	if p.Key() == r.Key() {
		t.Error("ParamPath and RetPath should have different keys")
	}
}

func TestPathHelpers_Consistency(t *testing.T) {
	// Multiple calls should return consistent values
	for i := 0; i < 10; i++ {
		p1 := ParamPath(i)
		p2 := ParamPath(i)
		if p1.Root != p2.Root {
			t.Errorf("ParamPath(%d) not consistent", i)
		}

		r1 := RetPath(i)
		r2 := RetPath(i)
		if r1.Root != r2.Root {
			t.Errorf("RetPath(%d) not consistent", i)
		}
	}
}

func TestPathHelpers_NegativeIndicesRejected(t *testing.T) {
	if p := ParamPath(-1); !p.IsEmpty() {
		t.Fatalf("ParamPath(-1) should be empty path, got %+v", p)
	}
	if p := RetPath(-1); !p.IsEmpty() {
		t.Fatalf("RetPath(-1) should be empty path, got %+v", p)
	}
}

func TestPathHelpers_UniqueKeys(t *testing.T) {
	seen := make(map[PathKey]string)
	paths := []struct {
		name string
		path Path
	}{
		{"param0", ParamPath(0)},
		{"param1", ParamPath(1)},
		{"param2", ParamPath(2)},
		{"ret0", RetPath(0)},
		{"ret1", RetPath(1)},
		{"ret2", RetPath(2)},
	}

	for _, p := range paths {
		key := p.path.Key()
		if existing, ok := seen[key]; ok {
			t.Errorf("Key collision: %s and %s both have key %q", existing, p.name, key)
		}
		seen[key] = p.name
	}
}

func TestPlaceholderIndexFromString(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
	}{
		{"$0", 0},
		{"$42", 42},
		{"param[0]", 0},
		{"param[42]", 42},
		{"$-1", -1},
		{"param[-1]", -1},
		{"$", -1},
		{"param[]", -1},
		{"param[abc]", -1},
		{"param[1", -1},
		{"x", -1},
		{"$" + overflow, -1},
		{"param[" + overflow + "]", -1},
	}

	for _, tc := range tests {
		if got := PlaceholderIndexFromString(tc.input); got != tc.want {
			t.Errorf("PlaceholderIndexFromString(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestReturnIndexFromString(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	overflow := strconv.FormatInt(int64(maxInt), 10) + "0"

	tests := []struct {
		input string
		want  int
	}{
		{"ret[0]", 0},
		{"ret[42]", 42},
		{"ret[-1]", -1},
		{"ret[]", -1},
		{"ret[abc]", -1},
		{"ret[1", -1},
		{"ret1]", -1},
		{"x", -1},
		{"ret[" + overflow + "]", -1},
	}

	for _, tc := range tests {
		if got := ReturnIndexFromString(tc.input); got != tc.want {
			t.Errorf("ReturnIndexFromString(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestPlaceholderArgIndex(t *testing.T) {
	tests := []struct {
		name     string
		path     Path
		argCount int
		wantIdx  int
		wantOK   bool
	}{
		{name: "placeholder in range", path: ParamPath(0), argCount: 1, wantIdx: 0, wantOK: true},
		{name: "placeholder out of range", path: ParamPath(1), argCount: 1, wantOK: false},
		{name: "non-placeholder path", path: Path{Root: "x"}, argCount: 1, wantOK: false},
		{name: "empty arg list", path: ParamPath(0), argCount: 0, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIdx, gotOK := PlaceholderArgIndex(tt.path, tt.argCount)
			if gotOK != tt.wantOK {
				t.Fatalf("PlaceholderArgIndex(%s, %d) ok = %v, want %v", tt.path, tt.argCount, gotOK, tt.wantOK)
			}
			if gotOK && gotIdx != tt.wantIdx {
				t.Fatalf("PlaceholderArgIndex(%s, %d) idx = %d, want %d", tt.path, tt.argCount, gotIdx, tt.wantIdx)
			}
		})
	}
}
