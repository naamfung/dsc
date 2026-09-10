package lua

import (
	"unicode/utf8"
)

func OpenUtf8(L *LState) int {
	mod := L.RegisterGoModule(Utf8LibName, utf8Funcs).(*LTable)
	mod.RawSetString("charpattern", LString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"))
	L.Push(mod)
	return 1
}

var utf8Funcs = map[string]LGoFunc{
	"char":      utf8Char,
	"codes":     utf8Codes,
	"codepoint": utf8Codepoint,
	"len":       utf8Len,
	"offset":    utf8Offset,
}

func utf8Char(L *LState) int {
	n := L.GetTop()
	buf := make([]byte, 0, n*4)
	for i := 1; i <= n; i++ {
		cp := L.CheckInt(i)
		if cp < 0 || cp > 0x10FFFF {
			L.ArgError(i, "value out of range")
		}
		var tmp [4]byte
		size := utf8.EncodeRune(tmp[:], rune(cp))
		buf = append(buf, tmp[:size]...)
	}
	L.Push(LString(buf))
	return 1
}

func utf8Codes(L *LState) int {
	s := L.CheckString(1)
	L.Push(LGoFunc(utf8CodesIter))
	L.Push(LString(s))
	L.Push(LInteger(0))
	return 3
}

func utf8CodesIter(L *LState) int {
	s := L.CheckString(1)
	pos := L.CheckInt(2)

	if pos >= len(s) {
		return 0
	}

	r, size := utf8.DecodeRuneInString(s[pos:])
	if r == utf8.RuneError && size == 1 {
		L.RaiseError("invalid UTF-8 code")
	}

	L.Push(LInteger(pos + 1))
	L.Push(LInteger(r))
	return 2
}

func utf8Codepoint(L *LState) int {
	s := L.CheckString(1)
	i := L.OptInt(2, 1)
	j := L.OptInt(3, i)

	if i < 1 {
		i = 1
	}
	if j > len(s) {
		j = len(s)
	}
	if i > j {
		return 0
	}

	i-- // convert to 0-based
	j--

	count := 0
	for pos := 0; pos < len(s); {
		r, size := utf8.DecodeRuneInString(s[pos:])
		if r == utf8.RuneError && size == 1 {
			L.RaiseError("invalid UTF-8 code")
		}
		if pos >= i && pos <= j {
			L.Push(LInteger(r))
			count++
		}
		pos += size
		if pos > j {
			break
		}
	}
	return count
}

func utf8Len(L *LState) int {
	s := L.CheckString(1)
	i := L.OptInt(2, 1)
	j := L.OptInt(3, -1)

	slen := len(s)
	if i < 0 {
		i = slen + i + 1
	}
	if j < 0 {
		j = slen + j + 1
	}
	if i < 1 {
		i = 1
	}
	if j > slen {
		j = slen
	}

	if i > j {
		L.Push(LInteger(0))
		return 1
	}

	sub := s[i-1 : j]
	count := 0
	for pos := 0; pos < len(sub); {
		r, size := utf8.DecodeRuneInString(sub[pos:])
		if r == utf8.RuneError && size == 1 {
			L.Push(LNil)
			L.Push(LInteger(i + pos))
			return 2
		}
		count++
		pos += size
	}
	L.Push(LInteger(count))
	return 1
}

func utf8Offset(L *LState) int {
	s := L.CheckString(1)
	n := L.CheckInt(2)
	i := L.OptInt(3, 1)

	slen := len(s)
	if i < 0 {
		i = slen + i + 2
	}
	if i < 1 {
		i = 1
	}
	if i > slen+1 {
		i = slen + 1
	}

	pos := i - 1

	if n == 0 {
		for pos > 0 && isContinuation(s[pos]) {
			pos--
		}
		L.Push(LInteger(pos + 1))
		return 1
	}

	if n > 0 {
		n--
		for n > 0 && pos < slen {
			pos++
			for pos < slen && isContinuation(s[pos]) {
				pos++
			}
			n--
		}
		if n == 0 {
			L.Push(LInteger(pos + 1))
			return 1
		}
	} else {
		for n < 0 && pos > 0 {
			pos--
			for pos > 0 && isContinuation(s[pos]) {
				pos--
			}
			n++
		}
		if n == 0 {
			L.Push(LInteger(pos + 1))
			return 1
		}
	}

	L.Push(LNil)
	return 1
}

func isContinuation(b byte) bool {
	return b&0xC0 == 0x80
}
