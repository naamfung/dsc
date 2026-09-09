package lua

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/wippyai/go-lua/pm"
)

const emptyLString = LString("")

var strFuncs = map[string]LGoFunc{
	"byte":     strByte,
	"char":     strChar,
	"dump":     strDump,
	"find":     strFind,
	"format":   strFormat,
	"gsub":     strGsub,
	"len":      strLen,
	"lower":    strLower,
	"match":    strMatch,
	"pack":     strPack,
	"packsize": strPacksize,
	"rep":      strRep,
	"reverse":  strReverse,
	"sub":      strSub,
	"unpack":   strUnpack,
	"upper":    strUpper,
}

func OpenString(L *LState) int {
	mod := L.RegisterGoModule(StringLibName, strFuncs).(*LTable)
	gmatch := L.NewClosure(strGmatch, L.NewFunction(strGmatchIter))
	mod.RawSetString("gmatch", gmatch)
	mod.RawSetString("gfind", gmatch)
	mod.RawSetString("__index", mod)
	L.G.builtinMts[int(LTString)] = mod
	L.Push(mod)
	return 1
}

func strByte(L *LState) int {
	str := L.CheckString(1)
	start := L.OptInt(2, 1) - 1
	end := L.OptInt(3, -1)
	l := len(str)
	if start < 0 {
		start = l + start + 1
	}
	if end < 0 {
		end = l + end + 1
	}

	if L.GetTop() == 2 {
		if start < 0 || start >= l {
			return 0
		}
		L.Push(LNumber(str[start]))
		return 1
	}

	start = intMax(start, 0)
	end = intMin(end, l)
	if end < 0 || end <= start || start >= l {
		return 0
	}

	for i := start; i < end; i++ {
		L.Push(LNumber(str[i]))
	}
	return end - start
}

func strChar(L *LState) int {
	top := L.GetTop()
	bytes := make([]byte, L.GetTop())
	for i := 1; i <= top; i++ {
		bytes[i-1] = uint8(L.CheckInt(i))
	}
	L.Push(LString(bytes))
	return 1
}

func strDump(L *LState) int {
	L.RaiseError("GopherLua does not support the string.dump")
	return 0
}

func strFind(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	if len(pattern) == 0 {
		L.Push(LNumber(1))
		L.Push(LNumber(0))
		return 2
	}
	init := luaIndex2StringIndex(str, L.OptInt(3, 1), true)
	plain := false
	if L.GetTop() == 4 {
		plain = LVAsBool(L.Get(4))
	}

	if plain {
		pos := strings.Index(str[init:], pattern)
		if pos < 0 {
			L.Push(LNil)
			return 1
		}
		L.Push(LNumber(init+pos) + 1)
		L.Push(LNumber(init + pos + len(pattern)))
		return 2
	}

	mds, err := pm.Find(pattern, unsafeFastStringToReadOnlyBytes(str), init, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 1
	}
	md := mds[0]
	L.Push(LNumber(md.Capture(0) + 1))
	L.Push(LNumber(md.Capture(1)))
	for i := 2; i < md.CaptureLength(); i += 2 {
		if md.IsPosCapture(i) {
			L.Push(LNumber(md.Capture(i)))
		} else {
			L.Push(LString(str[md.Capture(i):md.Capture(i+1)]))
		}
	}
	return md.CaptureLength()/2 + 1
}

func strFormat(L *LState) int {
	str := L.CheckString(1)
	args := make([]any, L.GetTop()-1)
	top := L.GetTop()
	for i := 2; i <= top; i++ {
		args[i-2] = L.Get(i)
	}
	npat := strings.Count(str, "%") - strings.Count(str, "%%")
	L.Push(LString(fmt.Sprintf(str, args[:intMin(npat, len(args))]...)))
	return 1
}

func strGsub(L *LState) int {
	str := L.CheckString(1)
	pat := L.CheckString(2)
	L.CheckTypes(3, LTString, LTTable, LTFunction)
	repl := L.CheckAny(3)
	limit := L.OptInt(4, -1)

	mds, err := pm.Find(pat, unsafeFastStringToReadOnlyBytes(str), 0, limit)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.SetTop(1)
		L.Push(LNumber(0))
		return 2
	}
	switch lv := repl.(type) {
	case LString:
		L.Push(LString(strGsubStr(L, str, string(lv), mds)))
	case *LTable:
		L.Push(LString(strGsubTable(L, str, lv, mds)))
	case *LFunction:
		L.Push(LString(strGsubFunc(L, str, lv, mds)))
	}
	L.Push(LNumber(len(mds)))
	return 2
}

type replaceInfo struct {
	Indicies []int
	String   string
}

func checkCaptureIndex(L *LState, m *pm.MatchData, idx int) {
	if idx <= 2 {
		return
	}
	if idx >= m.CaptureLength() {
		L.RaiseError("invalid capture index")
	}
}

func capturedString(L *LState, m *pm.MatchData, str string, idx int) string {
	checkCaptureIndex(L, m, idx)
	if idx >= m.CaptureLength() && idx == 2 {
		idx = 0
	}
	if m.IsPosCapture(idx) {
		return fmt.Sprint(m.Capture(idx))
	}
	return str[m.Capture(idx):m.Capture(idx+1)]

}

func strGsubDoReplace(str string, info []replaceInfo) string {
	offset := 0
	buf := []byte(str)
	for _, replace := range info {
		oldlen := len(buf)
		b1 := append([]byte(""), buf[0:offset+replace.Indicies[0]]...)
		b2 := []byte("")
		index2 := offset + replace.Indicies[1]
		if index2 <= len(buf) {
			b2 = append(b2, buf[index2:]...)
		}
		buf = append(b1, replace.String...)
		buf = append(buf, b2...)
		offset += len(buf) - oldlen
	}
	return string(buf)
}

func strGsubStr(L *LState, str string, repl string, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		start, end := match.Capture(0), match.Capture(1)
		sc := newFlagScanner('%', "", "", repl)
		for c, eos := sc.Next(); !eos; c, eos = sc.Next() {
			if !sc.ChangeFlag {
				if sc.HasFlag {
					if c >= '0' && c <= '9' {
						sc.AppendString(capturedString(L, match, str, 2*(int(c)-48)))
					} else {
						sc.AppendChar('%')
						sc.AppendChar(c)
					}
					sc.HasFlag = false
				} else {
					sc.AppendChar(c)
				}
			}
		}
		infoList = append(infoList, replaceInfo{[]int{start, end}, sc.String()})
	}

	return strGsubDoReplace(str, infoList)
}

func strGsubTable(L *LState, str string, repl *LTable, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		idx := 0
		if match.CaptureLength() > 2 { // has captures
			idx = 2
		}
		var value LValue
		if match.IsPosCapture(idx) {
			value = L.GetTable(repl, LNumber(match.Capture(idx)))
		} else {
			value = L.GetField(repl, str[match.Capture(idx):match.Capture(idx+1)])
		}
		if !LVIsFalse(value) {
			infoList = append(infoList, replaceInfo{[]int{match.Capture(0), match.Capture(1)}, LVAsString(value)})
		}
	}
	return strGsubDoReplace(str, infoList)
}

func strGsubFunc(L *LState, str string, repl *LFunction, matches []*pm.MatchData) string {
	infoList := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		start, end := match.Capture(0), match.Capture(1)
		L.Push(repl)
		nargs := 0
		if match.CaptureLength() > 2 { // has captures
			for i := 2; i < match.CaptureLength(); i += 2 {
				if match.IsPosCapture(i) {
					L.Push(LNumber(match.Capture(i)))
				} else {
					L.Push(LString(capturedString(L, match, str, i)))
				}
				nargs++
			}
		} else {
			L.Push(LString(capturedString(L, match, str, 0)))
			nargs++
		}
		L.Call(nargs, 1)
		ret := L.reg.Pop()
		if !LVIsFalse(ret) {
			infoList = append(infoList, replaceInfo{[]int{start, end}, LVAsString(ret)})
		}
	}
	return strGsubDoReplace(str, infoList)
}

type strMatchData struct {
	str     string
	pos     int
	matches []*pm.MatchData
}

func strGmatchIter(L *LState) int {
	md := L.CheckUserData(1).Value.(*strMatchData)
	str := md.str
	matches := md.matches
	idx := md.pos
	md.pos++
	if idx == len(matches) {
		return 0
	}
	L.Push(L.Get(1))
	match := matches[idx]
	if match.CaptureLength() == 2 {
		L.Push(LString(str[match.Capture(0):match.Capture(1)]))
		return 1
	}

	for i := 2; i < match.CaptureLength(); i += 2 {
		if match.IsPosCapture(i) {
			L.Push(LNumber(match.Capture(i)))
		} else {
			L.Push(LString(str[match.Capture(i):match.Capture(i+1)]))
		}
	}
	return match.CaptureLength()/2 - 1
}

func strGmatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	mds, err := pm.Find(pattern, []byte(str), 0, -1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	L.Push(L.Get(UpvalueIndex(1)))
	ud := L.NewUserData()
	ud.Value = &strMatchData{str, 0, mds}
	L.Push(ud)
	return 2
}

func strLen(L *LState) int {
	str := L.CheckString(1)
	L.Push(LNumber(len(str)))
	return 1
}

func strLower(L *LState) int {
	str := L.CheckString(1)
	L.Push(LString(strings.ToLower(str)))
	return 1
}

func strMatch(L *LState) int {
	str := L.CheckString(1)
	pattern := L.CheckString(2)
	offset := L.OptInt(3, 1)
	l := len(str)
	if offset < 0 {
		offset = l + offset + 1
	}
	offset--
	if offset < 0 {
		offset = 0
	}

	mds, err := pm.Find(pattern, unsafeFastStringToReadOnlyBytes(str), offset, 1)
	if err != nil {
		L.RaiseError(err.Error())
	}
	if len(mds) == 0 {
		L.Push(LNil)
		return 0
	}
	md := mds[0]
	nsubs := md.CaptureLength() / 2
	switch nsubs {
	case 1:
		L.Push(LString(str[md.Capture(0):md.Capture(1)]))
		return 1
	default:
		for i := 2; i < md.CaptureLength(); i += 2 {
			if md.IsPosCapture(i) {
				L.Push(LNumber(md.Capture(i)))
			} else {
				L.Push(LString(str[md.Capture(i):md.Capture(i+1)]))
			}
		}
		return nsubs - 1
	}
}

func strRep(L *LState) int {
	str := L.CheckString(1)
	n := L.CheckInt(2)
	if n < 0 {
		L.Push(emptyLString)
	} else {
		L.Push(LString(strings.Repeat(str, n)))
	}
	return 1
}

func strReverse(L *LState) int {
	str := L.CheckString(1)
	bts := []byte(str)
	out := make([]byte, len(bts))
	for i, j := 0, len(bts)-1; j >= 0; i, j = i+1, j-1 {
		out[i] = bts[j]
	}
	L.Push(LString(out))
	return 1
}

func strSub(L *LState) int {
	str := L.CheckString(1)
	start := luaIndex2StringIndex(str, L.CheckInt(2), true)
	end := luaIndex2StringIndex(str, L.OptInt(3, -1), false)
	l := len(str)
	if start >= l || end < start {
		L.Push(emptyLString)
	} else {
		// Clone so the result owns its bytes; a raw slice would keep the
		// entire source string's backing array alive for the substring's lifetime.
		L.Push(LString(strings.Clone(str[start:end])))
	}
	return 1
}

func strUpper(L *LState) int {
	str := L.CheckString(1)
	L.Push(LString(strings.ToUpper(str)))
	return 1
}

func luaIndex2StringIndex(str string, i int, start bool) int {
	if start && i != 0 {
		i -= 1
	}
	l := len(str)
	if i < 0 {
		i = l + i + 1
	}
	i = intMax(0, i)
	if !start && i > l {
		i = l
	}
	return i
}

type packState struct {
	fmt       string
	pos       int
	arg       int
	byteOrder binary.ByteOrder
	maxAlign  int
}

func (ps *packState) next() (byte, bool) {
	for ps.pos < len(ps.fmt) {
		c := ps.fmt[ps.pos]
		ps.pos++
		if c != ' ' {
			return c, true
		}
	}
	return 0, false
}

func (ps *packState) getNum(def int) int {
	if ps.pos >= len(ps.fmt) {
		return def
	}
	if ps.fmt[ps.pos] < '0' || ps.fmt[ps.pos] > '9' {
		return def
	}
	n := 0
	for ps.pos < len(ps.fmt) && ps.fmt[ps.pos] >= '0' && ps.fmt[ps.pos] <= '9' {
		n = n*10 + int(ps.fmt[ps.pos]-'0')
		ps.pos++
	}
	return n
}

func strPack(L *LState) int {
	fmtStr := L.CheckString(1)
	ps := &packState{
		fmt:       fmtStr,
		pos:       0,
		arg:       2,
		byteOrder: nativeByteOrder(),
		maxAlign:  1,
	}

	var buf []byte

	for {
		c, ok := ps.next()
		if !ok {
			break
		}

		switch c {
		case '<':
			ps.byteOrder = binary.LittleEndian
		case '>':
			ps.byteOrder = binary.BigEndian
		case '=':
			ps.byteOrder = nativeByteOrder()
		case '!':
			ps.maxAlign = ps.getNum(maxIntSize())

		case 'b':
			v := L.CheckInt(ps.arg)
			ps.arg++
			buf = append(buf, byte(int8(v)))

		case 'B':
			v := L.CheckInt(ps.arg)
			ps.arg++
			buf = append(buf, byte(v))

		case 'h':
			v := L.CheckInt(ps.arg)
			ps.arg++
			b := make([]byte, 2)
			ps.byteOrder.PutUint16(b, uint16(int16(v)))
			buf = append(buf, b...)

		case 'H':
			v := L.CheckInt(ps.arg)
			ps.arg++
			b := make([]byte, 2)
			ps.byteOrder.PutUint16(b, uint16(v))
			buf = append(buf, b...)

		case 'l', 'j':
			v := L.CheckInt64(ps.arg)
			ps.arg++
			b := make([]byte, 8)
			ps.byteOrder.PutUint64(b, uint64(v))
			buf = append(buf, b...)

		case 'L', 'J', 'T':
			v := uint64(L.CheckInt64(ps.arg))
			ps.arg++
			b := make([]byte, 8)
			ps.byteOrder.PutUint64(b, v)
			buf = append(buf, b...)

		case 'i':
			size := ps.getNum(4)
			v := L.CheckInt64(ps.arg)
			ps.arg++
			buf = appendIntN(buf, v, size, ps.byteOrder)

		case 'I':
			size := ps.getNum(4)
			v := uint64(L.CheckInt64(ps.arg))
			ps.arg++
			buf = appendUintN(buf, v, size, ps.byteOrder)

		case 'f':
			v := float32(L.CheckNumber(ps.arg))
			ps.arg++
			b := make([]byte, 4)
			ps.byteOrder.PutUint32(b, math.Float32bits(v))
			buf = append(buf, b...)

		case 'd', 'n':
			v := float64(L.CheckNumber(ps.arg))
			ps.arg++
			b := make([]byte, 8)
			ps.byteOrder.PutUint64(b, math.Float64bits(v))
			buf = append(buf, b...)

		case 'c':
			size := ps.getNum(0)
			s := L.CheckString(ps.arg)
			ps.arg++
			if len(s) < size {
				s = s + strings.Repeat("\x00", size-len(s))
			}
			buf = append(buf, s[:size]...)

		case 'z':
			s := L.CheckString(ps.arg)
			ps.arg++
			buf = append(buf, s...)
			buf = append(buf, 0)

		case 's':
			size := ps.getNum(8)
			s := L.CheckString(ps.arg)
			ps.arg++
			buf = appendUintN(buf, uint64(len(s)), size, ps.byteOrder)
			buf = append(buf, s...)

		case 'x':
			buf = append(buf, 0)

		case 'X':
			// alignment only, handled by Xop

		default:
			L.RaiseError("invalid format option '%c'", c)
		}
	}

	L.Push(LString(buf))
	return 1
}

func strUnpack(L *LState) int {
	fmtStr := L.CheckString(1)
	data := L.CheckString(2)
	pos := L.OptInt(3, 1) - 1

	ps := &packState{
		fmt:       fmtStr,
		pos:       0,
		byteOrder: nativeByteOrder(),
		maxAlign:  1,
	}

	results := 0

	for {
		c, ok := ps.next()
		if !ok {
			break
		}

		switch c {
		case '<':
			ps.byteOrder = binary.LittleEndian
		case '>':
			ps.byteOrder = binary.BigEndian
		case '=':
			ps.byteOrder = nativeByteOrder()
		case '!':
			ps.maxAlign = ps.getNum(maxIntSize())

		case 'b':
			if pos >= len(data) {
				L.RaiseError("data string too short")
			}
			L.Push(LInteger(int8(data[pos])))
			pos++
			results++

		case 'B':
			if pos >= len(data) {
				L.RaiseError("data string too short")
			}
			L.Push(LInteger(data[pos]))
			pos++
			results++

		case 'h':
			if pos+2 > len(data) {
				L.RaiseError("data string too short")
			}
			v := int16(ps.byteOrder.Uint16([]byte(data[pos:])))
			L.Push(LInteger(v))
			pos += 2
			results++

		case 'H':
			if pos+2 > len(data) {
				L.RaiseError("data string too short")
			}
			v := ps.byteOrder.Uint16([]byte(data[pos:]))
			L.Push(LInteger(v))
			pos += 2
			results++

		case 'l', 'j':
			if pos+8 > len(data) {
				L.RaiseError("data string too short")
			}
			v := int64(ps.byteOrder.Uint64([]byte(data[pos:])))
			L.Push(LInteger(v))
			pos += 8
			results++

		case 'L', 'J', 'T':
			if pos+8 > len(data) {
				L.RaiseError("data string too short")
			}
			v := ps.byteOrder.Uint64([]byte(data[pos:]))
			L.Push(LInteger(int64(v)))
			pos += 8
			results++

		case 'i':
			size := ps.getNum(4)
			if pos+size > len(data) {
				L.RaiseError("data string too short")
			}
			v := readIntN([]byte(data[pos:]), size, ps.byteOrder)
			L.Push(LInteger(v))
			pos += size
			results++

		case 'I':
			size := ps.getNum(4)
			if pos+size > len(data) {
				L.RaiseError("data string too short")
			}
			v := readUintN([]byte(data[pos:]), size, ps.byteOrder)
			L.Push(LInteger(int64(v)))
			pos += size
			results++

		case 'f':
			if pos+4 > len(data) {
				L.RaiseError("data string too short")
			}
			bits := ps.byteOrder.Uint32([]byte(data[pos:]))
			L.Push(LNumber(math.Float32frombits(bits)))
			pos += 4
			results++

		case 'd', 'n':
			if pos+8 > len(data) {
				L.RaiseError("data string too short")
			}
			bits := ps.byteOrder.Uint64([]byte(data[pos:]))
			L.Push(LNumber(math.Float64frombits(bits)))
			pos += 8
			results++

		case 'c':
			size := ps.getNum(0)
			if pos+size > len(data) {
				L.RaiseError("data string too short")
			}
			L.Push(LString(data[pos : pos+size]))
			pos += size
			results++

		case 'z':
			end := pos
			for end < len(data) && data[end] != 0 {
				end++
			}
			if end >= len(data) {
				L.RaiseError("unfinished string for format 'z'")
			}
			L.Push(LString(data[pos:end]))
			pos = end + 1
			results++

		case 's':
			size := ps.getNum(8)
			if pos+size > len(data) {
				L.RaiseError("data string too short")
			}
			strLen := int(readUintN([]byte(data[pos:]), size, ps.byteOrder))
			pos += size
			if pos+strLen > len(data) {
				L.RaiseError("data string too short")
			}
			L.Push(LString(data[pos : pos+strLen]))
			pos += strLen
			results++

		case 'x':
			pos++

		case 'X':
			// alignment only

		default:
			L.RaiseError("invalid format option '%c'", c)
		}
	}

	L.Push(LInteger(pos + 1))
	return results + 1
}

func strPacksize(L *LState) int {
	fmtStr := L.CheckString(1)
	ps := &packState{
		fmt:       fmtStr,
		pos:       0,
		byteOrder: nativeByteOrder(),
		maxAlign:  1,
	}

	size := 0

	for {
		c, ok := ps.next()
		if !ok {
			break
		}

		switch c {
		case '<', '>', '=':
			// endianness, no size
		case '!':
			ps.getNum(maxIntSize())
		case 'b', 'B':
			size++
		case 'h', 'H':
			size += 2
		case 'l', 'L', 'j', 'J', 'T':
			size += 8
		case 'i', 'I':
			size += ps.getNum(4)
		case 'f':
			size += 4
		case 'd', 'n':
			size += 8
		case 'c':
			size += ps.getNum(0)
		case 'x':
			size++
		case 'X':
			// alignment
		case 'z', 's':
			L.RaiseError("variable-length format")
		default:
			L.RaiseError("invalid format option '%c'", c)
		}
	}

	L.Push(LInteger(size))
	return 1
}

func nativeByteOrder() binary.ByteOrder {
	return binary.LittleEndian
}

func maxIntSize() int {
	return 8
}

func appendIntN(buf []byte, v int64, n int, order binary.ByteOrder) []byte {
	b := make([]byte, 8)
	order.PutUint64(b, uint64(v))
	if order == binary.BigEndian {
		return append(buf, b[8-n:]...)
	}
	return append(buf, b[:n]...)
}

func appendUintN(buf []byte, v uint64, n int, order binary.ByteOrder) []byte {
	b := make([]byte, 8)
	order.PutUint64(b, v)
	if order == binary.BigEndian {
		return append(buf, b[8-n:]...)
	}
	return append(buf, b[:n]...)
}

func readIntN(b []byte, n int, order binary.ByteOrder) int64 {
	var buf [8]byte
	if order == binary.BigEndian {
		if b[0]&0x80 != 0 {
			for i := range buf {
				buf[i] = 0xff
			}
		}
		copy(buf[8-n:], b[:n])
	} else {
		copy(buf[:n], b[:n])
		if b[n-1]&0x80 != 0 {
			for i := n; i < 8; i++ {
				buf[i] = 0xff
			}
		}
	}
	return int64(order.Uint64(buf[:]))
}

func readUintN(b []byte, n int, order binary.ByteOrder) uint64 {
	var buf [8]byte
	if order == binary.BigEndian {
		copy(buf[8-n:], b[:n])
	} else {
		copy(buf[:n], b[:n])
	}
	return order.Uint64(buf[:])
}

//
