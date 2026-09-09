package pm

import (
	"testing"
)

func FuzzFind(f *testing.F) {
	seeds := []struct {
		pattern string
		input   string
	}{
		{"hello", "hello world"},
		{".", "abc"},
		{".+", "abc"},
		{".*", "abc"},
		{"%d+", "abc123def"},
		{"%a+", "hello123world"},
		{"[a-z]+", "Hello World"},
		{"^hello", "hello world"},
		{"world$", "hello world"},
		{"(hello)", "hello"},
		{"(%w+)", "hello world"},
		{"%b()", "f(x) = (a+b)"},
		{"(%d+)%1", "1212"},
		{"[^%s]+", "hello world"},
		{"%%", "100%"},
		{"%-", "a-b"},
		{"%[", "[bracket]"},
		{"", "empty pattern"},
		{"x", ""},
		{".*", ""},
		{"^$", ""},
	}

	for _, s := range seeds {
		f.Add(s.pattern, s.input)
	}

	f.Fuzz(func(t *testing.T, pattern, input string) {
		// Find now returns errors instead of panicking
		_, _ = Find(pattern, []byte(input), 0, -1)
	})
}

func FuzzFindWithOffset(f *testing.F) {
	f.Add("test", "test test test", 0, 10)
	f.Add("%w+", "hello world foo bar", 5, 2)
	f.Add(".", "abc", 1, 1)

	f.Fuzz(func(t *testing.T, pattern, input string, offset, limit int) {
		// Normalize inputs
		if offset < 0 {
			offset = 0
		}
		if offset > len(input) {
			offset = len(input)
		}
		if limit < -1 {
			limit = -1
		}

		_, _ = Find(pattern, []byte(input), offset, limit)
	})
}

func FuzzPatternCompile(f *testing.F) {
	patterns := []string{
		"hello",
		".",
		".+",
		".*",
		".?",
		".-",
		"%d",
		"%D",
		"%a",
		"%A",
		"%w",
		"%W",
		"%s",
		"%S",
		"%l",
		"%L",
		"%u",
		"%U",
		"%p",
		"%P",
		"%c",
		"%C",
		"%x",
		"%X",
		"%z",
		"%Z",
		"[abc]",
		"[^abc]",
		"[a-z]",
		"[A-Za-z0-9]",
		"^hello",
		"hello$",
		"^hello$",
		"(hello)",
		"(%w+)",
		"((a)(b))",
		"%b()",
		"%b[]",
		"%b{}",
		"(%d+)%1",
		"()",
		"%%",
		"%.",
		"%[",
		"%]",
		"%-",
	}

	for _, p := range patterns {
		f.Add(p)
	}

	f.Fuzz(func(t *testing.T, pattern string) {
		// parsePattern now returns errors instead of panicking
		sc := newScanner([]byte(pattern))
		_, _ = parsePattern(sc, true)
	})
}
