package pm

import (
	"testing"
)

func BenchmarkCompileSimple(b *testing.B) {
	pattern := "hello"
	for i := 0; i < b.N; i++ {
		pat, _ := parsePattern(newScanner([]byte(pattern)), true)
		_ = compilePattern(pat, nil)
	}
}

func BenchmarkCompileComplex(b *testing.B) {
	pattern := "(%a+)%s*=%s*(%d+)"
	for i := 0; i < b.N; i++ {
		pat, _ := parsePattern(newScanner([]byte(pattern)), true)
		_ = compilePattern(pat, nil)
	}
}

func BenchmarkCompileRepeat(b *testing.B) {
	pattern := "%w*%d+%s*"
	for i := 0; i < b.N; i++ {
		pat, _ := parsePattern(newScanner([]byte(pattern)), true)
		_ = compilePattern(pat, nil)
	}
}

func BenchmarkFind_Simple(b *testing.B) {
	src := []byte("hello world hello")
	pattern := "hello"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_Digits(b *testing.B) {
	src := []byte("test123abc456def789")
	pattern := "%d+"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_StarRepeat(b *testing.B) {
	src := []byte("aaaaaaaaaa")
	pattern := "a*"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_PlusRepeat(b *testing.B) {
	src := []byte("aaa bbb ccc ddd eee")
	pattern := "%a+"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_Capture(b *testing.B) {
	src := []byte("key1=value1 key2=value2 key3=value3")
	pattern := "(%w+)=(%w+)"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_MultiCapture(b *testing.B) {
	src := []byte("(a)(b)(c)(d)(e)")
	pattern := "%((.+)%)"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_Backreference(b *testing.B) {
	src := []byte("hello hello world world test test")
	pattern := "(%w+)%s+%1"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_CharacterClass(b *testing.B) {
	src := []byte("The quick brown fox jumps over the lazy dog 123")
	pattern := "[aeiou]+"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_LongText(b *testing.B) {
	src := []byte(`Lorem ipsum dolor sit amet, consectetur adipiscing elit.
	Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
	Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.
	Duis aute irure dolor in reprehenderit in voluptate velit esse cillum.
	Excepteur sint occaecat cupidatat non proident sunt in culpa qui officia.`)
	pattern := "%w+"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_DeepRecursion(b *testing.B) {
	src := []byte("aaaaaaaaaaaaaaaaaaaab")
	pattern := "a*a*a*a*a*b"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_ManyCaptures(b *testing.B) {
	src := []byte("a1 b2 c3 d4 e5 f6 g7 h8 i9 j0")
	pattern := "(%a)(%d)"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Find(pattern, src, 0, -1)
	}
}

func BenchmarkFind_SamePattern_MultipleInputs(b *testing.B) {
	inputs := [][]byte{
		[]byte("test123"),
		[]byte("abc456"),
		[]byte("xyz789"),
		[]byte("hello000"),
	}
	pattern := "%d+"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, input := range inputs {
			_, _ = Find(pattern, input, 0, -1)
		}
	}
}

func BenchmarkMatchCharClass(b *testing.B) {
	for i := 0; i < b.N; i++ {
		matchCharClass('w', 'a')
		matchCharClass('d', '5')
		matchCharClass('s', ' ')
		matchCharClass('A', 'x')
	}
}
