package pm

import (
	"testing"
)

func TestFind_Literal(t *testing.T) {
	matches, err := Find("hello", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_Dot(t *testing.T) {
	matches, err := Find("h.llo", []byte("hello hallo hullo"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_CharacterClass(t *testing.T) {
	matches, err := Find("%d+", []byte("abc123def456"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestFind_Capture(t *testing.T) {
	matches, err := Find("(%d+)", []byte("test123end"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.CaptureLength() < 4 {
		t.Fatalf("expected at least 4 captures, got %d", m.CaptureLength())
	}
	start := m.Capture(2)
	end := m.Capture(3)
	if start != 4 || end != 7 {
		t.Fatalf("expected capture at 4-7, got %d-%d", start, end)
	}
}

func TestFind_StarRepeat(t *testing.T) {
	matches, err := Find("a*b", []byte("b ab aaab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match, got %d", len(matches))
	}
}

func TestFind_PlusRepeat(t *testing.T) {
	matches, err := Find("a+", []byte("a aa aaa"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_MinusRepeat(t *testing.T) {
	matches, err := Find("a-b", []byte("aaab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_QuestionRepeat(t *testing.T) {
	matches, err := Find("a?b", []byte("b ab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestFind_AnchorStart(t *testing.T) {
	matches, err := Find("^hello", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("^hello", []byte("say hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_AnchorEnd(t *testing.T) {
	matches, err := Find("world$", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("world$", []byte("world hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_CharacterClasses(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		expect  int
	}{
		{"%a+", "abc123", 1},
		{"%d+", "abc123def", 1},
		{"%s+", "a b c", 2},
		{"%w+", "hello world!", 2},
		{"%l+", "Hello", 1},
		{"%u+", "Hello", 1},
	}

	for _, tt := range tests {
		matches, err := Find(tt.pattern, []byte(tt.input), 0, -1)
		if err != nil {
			t.Fatalf("pattern %q on %q: unexpected error: %v", tt.pattern, tt.input, err)
		}
		if len(matches) != tt.expect {
			t.Fatalf("pattern %q on %q: expected %d matches, got %d", tt.pattern, tt.input, tt.expect, len(matches))
		}
	}
}

func TestFind_Set(t *testing.T) {
	matches, err := Find("[aeiou]+", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_NegatedSet(t *testing.T) {
	matches, err := Find("[^aeiou]+", []byte("hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match, got %d", len(matches))
	}
}

func TestFind_Range(t *testing.T) {
	matches, err := Find("[a-z]+", []byte("Hello123"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_Backreference(t *testing.T) {
	matches, err := Find("(%a+)%s+%1", []byte("hello hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("(%a+)%s+%1", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_BalancedBraces(t *testing.T) {
	matches, err := Find("%b()", []byte("test (nested (braces)) here"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_PositionCapture(t *testing.T) {
	matches, err := Find("()test()", []byte("test"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.CaptureLength() < 6 {
		t.Fatalf("expected at least 6 captures for position captures, got %d", m.CaptureLength())
	}
}

func TestFind_Limit(t *testing.T) {
	matches, err := Find("%d", []byte("1234567890"), 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches (limited), got %d", len(matches))
	}
}

func TestFind_Offset(t *testing.T) {
	matches, err := Find("test", []byte("test test"), 5, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match (after offset), got %d", len(matches))
	}
}

func TestFind_InvalidPattern(t *testing.T) {
	_, err := Find("(unclosed", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for unclosed capture")
	}
}

func TestFind_InvalidCaptureIndex(t *testing.T) {
	_, err := Find("%0", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for invalid capture index")
	}
}

func TestFind_UnmatchedParen(t *testing.T) {
	_, err := Find("test)", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for unmatched paren")
	}
}

func TestError_String(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantStr string
	}{
		{
			name:    "normal position",
			err:     &Error{Pos: 5, Message: "invalid pattern"},
			wantStr: "invalid pattern",
		},
		{
			name:    "EOS position",
			err:     &Error{Pos: eos, Message: "unexpected end"},
			wantStr: "unexpected end",
		},
		{
			name:    "unknown position",
			err:     &Error{Pos: unknownPos, Message: "unknown error"},
			wantStr: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}

			result := "error: " + tt.err.String()
			if result != "error: "+tt.wantStr {
				t.Errorf("concatenation failed")
			}
		})
	}
}

func TestPatternCache_LRU(t *testing.T) {
	cache := newPatternCache(3)

	patterns := []string{"a", "b", "c", "d"}
	for _, p := range patterns {
		pat, err := parsePattern(newScanner([]byte(p)), true)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		insts := compilePattern(pat, nil)
		cache.put(p, insts, pat)
	}

	// "a" should have been evicted (LRU)
	if _, _, ok := cache.get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}

	// "b", "c", "d" should still be present
	for _, p := range []string{"b", "c", "d"} {
		if _, _, ok := cache.get(p); !ok {
			t.Errorf("expected %q to be in cache", p)
		}
	}
}

func TestMatchCharClass(t *testing.T) {
	tests := []struct {
		code  int
		ch    int
		match bool
	}{
		{'a', 'x', true},
		{'a', 'X', true},
		{'a', '1', false},
		{'A', 'x', false},
		{'A', '1', true},
		{'d', '5', true},
		{'d', 'a', false},
		{'D', '5', false},
		{'D', 'a', true},
		{'s', ' ', true},
		{'s', 'a', false},
		{'w', 'a', true},
		{'w', '5', true},
		{'w', ' ', false},
	}

	for _, tt := range tests {
		result := matchCharClass(tt.code, tt.ch)
		if result != tt.match {
			t.Errorf("matchCharClass(%c, %c) = %v, want %v", tt.code, tt.ch, result, tt.match)
		}
	}
}

func TestFind_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		wantN   int
		wantErr bool
	}{
		{"empty pattern", "", "test", 5, false},
		{"empty input", "a", "", 0, false},
		{"both empty", "", "", 1, false},
		{"trailing percent", "%", "test", 0, false},
		{"trailing backslash b", "%b", "test", 0, false},
		{"unclosed bracket", "[abc", "test", 0, true},
		{"deeply nested", "((((a))))", "a", 1, false},
		{"pattern too complex", string(make([]byte, 300)), "test", 0, true},
		{"dollar in middle", "a$b", "a$b", 1, false},
		{"caret in middle", "a^b", "a^b", 1, false},
		{"multiple anchors", "^test$", "test", 1, false},
		{"anchor no match", "^test$", "testing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a complex pattern for "pattern too complex" test
			pat := tt.pattern
			if tt.name == "pattern too complex" {
				pat = ""
				for i := 0; i < 250; i++ {
					pat += "("
				}
			}

			matches, err := Find(pat, []byte(tt.input), 0, -1)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(matches) != tt.wantN {
				t.Errorf("expected %d matches, got %d", tt.wantN, len(matches))
			}
		})
	}
}

func TestFind_MaxBacktracks(t *testing.T) {
	// Pattern that causes exponential backtracking
	pattern := "a*a*a*a*a*a*a*a*a*a*b"
	input := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 31 a's, no b

	matches, err := Find(pattern, []byte(input), 0, -1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_ConcurrentSafety(t *testing.T) {
	pattern := "%d+"
	inputs := []string{
		"test123abc",
		"456def789",
		"no digits here",
		"1",
		"123456789",
	}

	done := make(chan bool, len(inputs))
	for _, input := range inputs {
		go func(in string) {
			for i := 0; i < 100; i++ {
				_, _ = Find(pattern, []byte(in), 0, -1)
			}
			done <- true
		}(input)
	}

	for i := 0; i < len(inputs); i++ {
		<-done
	}
}

func TestMatchData_Bounds(t *testing.T) {
	md := &MatchData{captures: []uint32{0, 10, 20}}

	// Test out of bounds access
	if md.Capture(-1) != 0 {
		t.Error("Capture(-1) should return 0")
	}
	if md.Capture(100) != 0 {
		t.Error("Capture(100) should return 0")
	}
	if md.IsPosCapture(-1) {
		t.Error("IsPosCapture(-1) should return false")
	}
	if md.IsPosCapture(100) {
		t.Error("IsPosCapture(100) should return false")
	}
}
