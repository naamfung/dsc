// Package pm provides Lua pattern matching functions for Go.
package pm

import (
	"bytes"
	"container/list"
	"fmt"
	"sync"
)

const (
	eos         = -1
	unknownPos  = -2
	beforeStart = -3
)

const (
	// inlineCaptureCap is the threshold for inline vs heap capture storage.
	// 16 covers most patterns (up to 8 capture groups with start/end pairs).
	inlineCaptureCap = 16

	// maxPooledCapacity limits pooled slice sizes to prevent memory bloat.
	maxPooledCapacity = 128

	// initialStackCap is the initial capacity for the VM backtrack stack.
	initialStackCap = 8

	// maxPatternDepth limits recursion depth during parsing to prevent stack overflow.
	maxPatternDepth = 200
)

// Error represents a pattern matching error.
type Error struct {
	Pos     int
	Message string
}

func newError(pos int, message string, args ...any) *Error {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return &Error{Pos: pos, Message: message}
}

func (e *Error) Error() string {
	switch e.Pos {
	case eos:
		return fmt.Sprintf("%s at EOS", e.Message)
	case unknownPos:
		return e.Message
	default:
		return fmt.Sprintf("%s at %d", e.Message, e.Pos)
	}
}

func (e *Error) String() string {
	return e.Message
}

// MatchData holds captured positions from a match.
// Layout: bit 0 indicates position capture, bits 1+ hold the position value.
type MatchData struct {
	captures []uint32
}

var matchDataPool = sync.Pool{
	New: func() any {
		return &MatchData{captures: make([]uint32, 0, inlineCaptureCap)}
	},
}

func newMatchData(size int) *MatchData {
	md := matchDataPool.Get().(*MatchData)
	if cap(md.captures) < size {
		md.captures = make([]uint32, 0, size)
	} else {
		md.captures = md.captures[:0]
	}
	return md
}

func (md *MatchData) release() {
	if cap(md.captures) <= maxPooledCapacity {
		matchDataPool.Put(md)
	}
}

func (md *MatchData) addPosCapture(slot, pos int) {
	md.ensureSlot(slot + 1)
	md.captures[slot] = uint32(pos)<<1 | 1
	md.captures[slot+1] = uint32(pos)<<1 | 1
}

func (md *MatchData) setCapture(slot, pos int) {
	md.ensureSlot(slot)
	md.captures[slot] = uint32(pos) << 1
}

func (md *MatchData) ensureSlot(slot int) {
	for slot >= len(md.captures) {
		md.captures = append(md.captures, 0)
	}
}

func (md *MatchData) restoreFrom(t *vmThread) {
	if cap(md.captures) < t.captureLen {
		md.captures = make([]uint32, t.captureLen)
	} else {
		md.captures = md.captures[:t.captureLen]
	}
	if t.useInline {
		copy(md.captures, t.inlineCaptures[:t.captureLen])
	} else {
		copy(md.captures, t.heapCaptures[:t.captureLen])
	}
}

// CaptureLength returns the number of capture slots.
func (md *MatchData) CaptureLength() int { return len(md.captures) }

// IsPosCapture returns true if the capture at idx is a position capture.
func (md *MatchData) IsPosCapture(idx int) bool {
	if idx < 0 || idx >= len(md.captures) {
		return false
	}
	return (md.captures[idx] & 1) == 1
}

// Capture returns the captured position at idx.
func (md *MatchData) Capture(idx int) int {
	if idx < 0 || idx >= len(md.captures) {
		return 0
	}
	return int(md.captures[idx] >> 1)
}

// scanner tokenizes pattern input.
type scanner struct {
	src      []byte
	pos      int
	savedPos int
}

func newScanner(src []byte) *scanner {
	return &scanner{src: src, pos: beforeStart, savedPos: beforeStart}
}

func (sc *scanner) next() int {
	switch sc.pos {
	case beforeStart:
		sc.pos = 0
	case eos:
		return eos
	default:
		sc.pos++
	}
	if sc.pos >= len(sc.src) {
		sc.pos = eos
		return eos
	}
	return int(sc.src[sc.pos])
}

func (sc *scanner) currentPos() int {
	return sc.pos
}

func (sc *scanner) peek() int {
	if sc.pos == eos {
		return eos
	}
	var next int
	if sc.pos == beforeStart {
		next = 0
	} else {
		next = sc.pos + 1
	}
	if next >= len(sc.src) {
		return eos
	}
	return int(sc.src[next])
}

func (sc *scanner) atEnd() bool {
	if sc.pos == eos {
		return true
	}
	if sc.pos == beforeStart {
		return len(sc.src) == 0
	}
	return sc.pos+1 >= len(sc.src)
}

func (sc *scanner) save() {
	sc.savedPos = sc.pos
}

func (sc *scanner) restore() {
	sc.pos = sc.savedPos
}

// opCode represents a VM instruction type.
type opCode int

const (
	opChar      opCode = iota // match character against class
	opMatch                   // successful match
	opTailMatch               // match only if at end of input
	opJmp                     // unconditional jump
	opSplit                   // fork execution (backtrack point)
	opSave                    // save position to capture slot
	opPSave                   // save position capture (1-indexed)
	opBrace                   // balanced brace matching
	opNumber                  // backreference to capture group
)

type instruction struct {
	op       opCode
	class    charClass
	operand1 int
	operand2 int
}

// charClass matches a character against a pattern class.
type charClass interface {
	matches(ch int) bool
}

// dotClass matches any character (singleton).
var theDotClass = &dotClass{}

type dotClass struct{}

func (dc *dotClass) matches(_ int) bool { return true }

type literalClass struct {
	char int
}

func (lc *literalClass) matches(ch int) bool { return lc.char == ch }

type singleClass struct {
	code int
}

func (sc *singleClass) matches(ch int) bool {
	return matchCharClass(sc.code, ch)
}

// matchCharClass matches Lua character classes.
func matchCharClass(code, ch int) bool {
	var matched bool
	switch code {
	case 'a', 'A':
		matched = ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
	case 'c', 'C':
		matched = (0x00 <= ch && ch <= 0x1F) || ch == 0x7F
	case 'd', 'D':
		matched = '0' <= ch && ch <= '9'
	case 'l', 'L':
		matched = 'a' <= ch && ch <= 'z'
	case 'p', 'P':
		matched = (0x21 <= ch && ch <= 0x2f) || (0x3a <= ch && ch <= 0x40) ||
			(0x5b <= ch && ch <= 0x60) || (0x7b <= ch && ch <= 0x7e)
	case 's', 'S':
		switch ch {
		case ' ', '\f', '\n', '\r', '\t', '\v':
			matched = true
		}
	case 'u', 'U':
		matched = 'A' <= ch && ch <= 'Z'
	case 'w', 'W':
		matched = ('0' <= ch && ch <= '9') || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
	case 'x', 'X':
		matched = ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
	case 'z', 'Z':
		matched = ch == 0
	default:
		return ch == code
	}
	if 'A' <= code && code <= 'Z' {
		return !matched
	}
	return matched
}

type setClass struct {
	negated bool
	classes []charClass
}

func (sc *setClass) matches(ch int) bool {
	for _, cls := range sc.classes {
		if cls.matches(ch) {
			return !sc.negated
		}
	}
	return sc.negated
}

type rangeClass struct {
	begin int
	end   int
}

func (rc *rangeClass) matches(ch int) bool {
	return rc.begin <= ch && ch <= rc.end
}

// pattern is a parsed pattern node.
type pattern interface {
	patternNode()
}

type singlePattern struct {
	class charClass
}

func (*singlePattern) patternNode() {}

type seqPattern struct {
	mustHead bool
	mustTail bool
	patterns []pattern
}

func (*seqPattern) patternNode() {}

type repeatPattern struct {
	repeatType int
	class      charClass
}

func (*repeatPattern) patternNode() {}

type posCapPattern struct{}

func (*posCapPattern) patternNode() {}

type capPattern struct {
	inner pattern
}

func (*capPattern) patternNode() {}

type numberPattern struct {
	index int
}

func (*numberPattern) patternNode() {}

type bracePattern struct {
	begin int
	end   int
}

func (*bracePattern) patternNode() {}

func parseClass(sc *scanner, allowSet bool) (charClass, error) {
	ch := sc.next()
	switch ch {
	case '%':
		return &singleClass{sc.next()}, nil
	case '.':
		if allowSet {
			return theDotClass, nil
		}
		return &literalClass{ch}, nil
	case '[':
		if allowSet {
			return parseClassSet(sc)
		}
		return &literalClass{ch}, nil
	case eos:
		return nil, newError(sc.currentPos(), "unexpected EOS")
	default:
		return &literalClass{ch}, nil
	}
}

func parseClassSet(sc *scanner) (charClass, error) {
	set := &setClass{}
	if sc.peek() == '^' {
		set.negated = true
		sc.next()
	}

	pendingRange := false
	for {
		ch := sc.peek()

		// End of set
		if ch == ']' && len(set.classes) > 0 {
			sc.next()
			if pendingRange {
				set.classes = append(set.classes, &literalClass{'-'})
			}
			return set, nil
		}

		// EOS without closing bracket
		if ch == eos {
			return nil, newError(sc.currentPos(), "unexpected EOS")
		}

		// Range operator
		if ch == '-' && len(set.classes) > 0 && !pendingRange {
			sc.next()
			pendingRange = true
			continue
		}

		cls, err := parseClass(sc, false)
		if err != nil {
			return nil, err
		}

		if pendingRange {
			prev := set.classes[len(set.classes)-1]
			set.classes = set.classes[:len(set.classes)-1]
			begin, end := extractRangeBounds(prev, cls)
			set.classes = append(set.classes, &rangeClass{begin, end})
			pendingRange = false
		} else {
			set.classes = append(set.classes, cls)
		}
	}
}

func extractRangeBounds(begin, end charClass) (int, int) {
	b, e := 0, 0
	if lit, ok := begin.(*literalClass); ok {
		b = lit.char
	}
	if lit, ok := end.(*literalClass); ok {
		e = lit.char
	}
	return b, e
}

func parsePattern(sc *scanner, topLevel bool) (*seqPattern, error) {
	return parsePatternDepth(sc, topLevel, 0)
}

func parsePatternDepth(sc *scanner, topLevel bool, depth int) (*seqPattern, error) {
	if depth > maxPatternDepth {
		return nil, newError(sc.currentPos(), "pattern too complex")
	}
	pat := &seqPattern{}
	if topLevel && sc.peek() == '^' {
		sc.next()
		pat.mustHead = true
	}
	for {
		ch := sc.peek()
		switch ch {
		case '%':
			if err := parseEscape(sc, pat); err != nil {
				return nil, err
			}
		case '.', '[', ']':
			cls, err := parseClass(sc, true)
			if err != nil {
				return nil, err
			}
			pat.patterns = append(pat.patterns, &singlePattern{cls})
		case ')':
			if topLevel {
				return nil, newError(sc.currentPos(), "invalid ')'")
			}
			return pat, nil
		case '(':
			if err := parseCaptureDepth(sc, pat, depth+1); err != nil {
				return nil, err
			}
		case '*', '+', '-', '?':
			parseRepeat(sc, pat, ch)
		case '$':
			sc.next()
			if topLevel && sc.atEnd() {
				pat.mustTail = true
			} else {
				pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
			}
		case eos:
			return pat, nil
		default:
			sc.next()
			pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
		}
	}
}

func parseEscape(sc *scanner, pat *seqPattern) error {
	sc.save()
	sc.next()
	switch sc.peek() {
	case '0':
		return newError(sc.currentPos(), "invalid capture index")
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		pat.patterns = append(pat.patterns, &numberPattern{sc.next() - '0'})
	case 'b':
		sc.next()
		pat.patterns = append(pat.patterns, &bracePattern{sc.next(), sc.next()})
	default:
		sc.restore()
		cls, err := parseClass(sc, true)
		if err != nil {
			return err
		}
		pat.patterns = append(pat.patterns, &singlePattern{cls})
	}
	return nil
}

func parseCaptureDepth(sc *scanner, pat *seqPattern, depth int) error {
	sc.next()
	if sc.peek() == ')' {
		sc.next()
		pat.patterns = append(pat.patterns, &posCapPattern{})
		return nil
	}
	inner, err := parsePatternDepth(sc, false, depth)
	if err != nil {
		return err
	}
	if sc.peek() != ')' {
		return newError(sc.currentPos(), "unfinished capture")
	}
	sc.next()
	pat.patterns = append(pat.patterns, &capPattern{inner})
	return nil
}

func parseRepeat(sc *scanner, pat *seqPattern, ch int) {
	sc.next()
	if len(pat.patterns) > 0 {
		if single, ok := pat.patterns[len(pat.patterns)-1].(*singlePattern); ok {
			pat.patterns[len(pat.patterns)-1] = &repeatPattern{ch, single.class}
			return
		}
	}
	pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
}

type instructionBuilder struct {
	instructions []instruction
	captureSlot  int
}

func compilePattern(p pattern, builder *instructionBuilder) []instruction {
	topLevel := builder == nil
	if topLevel {
		builder = &instructionBuilder{
			instructions: []instruction{{opSave, nil, 0, -1}},
			captureSlot:  2,
		}
	}

	switch pat := p.(type) {
	case *singlePattern:
		builder.instructions = append(builder.instructions, instruction{opChar, pat.class, -1, -1})

	case *seqPattern:
		for _, child := range pat.patterns {
			compilePattern(child, builder)
		}
		if topLevel {
			if pat.mustTail {
				builder.instructions = append(builder.instructions,
					instruction{opSave, nil, 1, -1},
					instruction{opTailMatch, nil, -1, -1})
			} else {
				builder.instructions = append(builder.instructions,
					instruction{opSave, nil, 1, -1},
					instruction{opMatch, nil, -1, -1})
			}
		}

	case *repeatPattern:
		compileRepeat(builder, pat)

	case *posCapPattern:
		builder.instructions = append(builder.instructions, instruction{opPSave, nil, builder.captureSlot, -1})
		builder.captureSlot += 2

	case *capPattern:
		startSlot := builder.captureSlot
		endSlot := builder.captureSlot + 1
		builder.captureSlot += 2
		builder.instructions = append(builder.instructions, instruction{opSave, nil, startSlot, -1})
		compilePattern(pat.inner, builder)
		builder.instructions = append(builder.instructions, instruction{opSave, nil, endSlot, -1})

	case *bracePattern:
		builder.instructions = append(builder.instructions, instruction{opBrace, nil, pat.begin, pat.end})

	case *numberPattern:
		builder.instructions = append(builder.instructions, instruction{opNumber, nil, pat.index, -1})
	}

	return builder.instructions
}

func compileRepeat(builder *instructionBuilder, pat *repeatPattern) {
	idx := len(builder.instructions)
	switch pat.repeatType {
	case '*':
		builder.instructions = append(builder.instructions,
			instruction{opSplit, nil, idx + 1, idx + 3},
			instruction{opChar, pat.class, -1, -1},
			instruction{opJmp, nil, idx, -1})
	case '+':
		builder.instructions = append(builder.instructions,
			instruction{opChar, pat.class, -1, -1},
			instruction{opSplit, nil, idx, idx + 2})
	case '-':
		builder.instructions = append(builder.instructions,
			instruction{opSplit, nil, idx + 3, idx + 1},
			instruction{opChar, pat.class, -1, -1},
			instruction{opJmp, nil, idx, -1})
	case '?':
		builder.instructions = append(builder.instructions,
			instruction{opSplit, nil, idx + 1, idx + 2},
			instruction{opChar, pat.class, -1, -1})
	}
}

// LRU pattern cache with bounded size.
type patternCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type cacheEntry struct {
	key   string
	insts []instruction
	pat   *seqPattern
}

func newPatternCache(capacity int) *patternCache {
	return &patternCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (pc *patternCache) get(pattern string) ([]instruction, *seqPattern, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	elem, ok := pc.items[pattern]
	if !ok {
		return nil, nil, false
	}
	pc.order.MoveToFront(elem)
	entry := elem.Value.(*cacheEntry)
	return entry.insts, entry.pat, true
}

func (pc *patternCache) put(pattern string, insts []instruction, pat *seqPattern) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if elem, ok := pc.items[pattern]; ok {
		pc.order.MoveToFront(elem)
		return
	}

	if pc.order.Len() >= pc.capacity {
		oldest := pc.order.Back()
		if oldest != nil {
			entry := oldest.Value.(*cacheEntry)
			delete(pc.items, entry.key)
			pc.order.Remove(oldest)
		}
	}

	entry := &cacheEntry{key: pattern, insts: insts, pat: pat}
	elem := pc.order.PushFront(entry)
	pc.items[pattern] = elem
}

const maxCachedPatterns = 256

var globalPatternCache = newPatternCache(maxCachedPatterns)

func getCompiledPattern(pattern string) ([]instruction, *seqPattern, error) {
	if insts, pat, ok := globalPatternCache.get(pattern); ok {
		return insts, pat, nil
	}

	pat, err := parsePattern(newScanner([]byte(pattern)), true)
	if err != nil {
		return nil, nil, err
	}
	insts := compilePattern(pat, nil)
	globalPatternCache.put(pattern, insts, pat)
	return insts, pat, nil
}

func calcMaxCaptureSlot(insts []instruction) int {
	maxSlot := 0
	for _, inst := range insts {
		if inst.op == opSave || inst.op == opPSave {
			if inst.operand1 > maxSlot {
				maxSlot = inst.operand1
			}
		}
	}
	return maxSlot + 2
}

// vmThread represents a backtrack point in the VM.
type vmThread struct {
	programCounter int
	sourcePos      int
	inlineCaptures [inlineCaptureCap]uint32
	heapCaptures   []uint32
	captureLen     int
	useInline      bool
}

var threadPool = sync.Pool{
	New: func() any {
		return &vmThread{}
	},
}

func newVMThread(programCounter, sourcePos int, captures []uint32) *vmThread {
	t := threadPool.Get().(*vmThread)
	t.programCounter = programCounter
	t.sourcePos = sourcePos
	t.captureLen = len(captures)

	if len(captures) <= inlineCaptureCap {
		t.useInline = true
		copy(t.inlineCaptures[:], captures)
	} else {
		t.useInline = false
		if cap(t.heapCaptures) < len(captures) {
			t.heapCaptures = make([]uint32, len(captures))
		} else {
			t.heapCaptures = t.heapCaptures[:len(captures)]
		}
		copy(t.heapCaptures, captures)
	}
	return t
}

func (t *vmThread) release() {
	if !t.useInline && cap(t.heapCaptures) > maxPooledCapacity {
		t.heapCaptures = nil
	}
	threadPool.Put(t)
}

// matchClass dispatches to the appropriate matcher with inlined hot paths.
func matchClass(cls charClass, ch int) bool {
	switch c := cls.(type) {
	case *literalClass:
		return c.char == ch
	case *singleClass:
		return matchCharClass(c.code, ch)
	case *dotClass:
		return true
	case *setClass:
		return c.matches(ch)
	case *rangeClass:
		return c.matches(ch)
	default:
		return cls.matches(ch)
	}
}

// MaxBacktracks limits backtracking to prevent ReDoS attacks.
const MaxBacktracks = 100000

type vm struct {
	src       []byte
	insts     []instruction
	matchData *MatchData
	stack     []*vmThread
}

var vmPool = sync.Pool{
	New: func() any {
		return &vm{stack: make([]*vmThread, 0, initialStackCap)}
	},
}

func newVM(src []byte, insts []instruction) *vm {
	v := vmPool.Get().(*vm)
	v.src = src
	v.insts = insts
	v.matchData = nil
	v.stack = v.stack[:0]
	return v
}

func (v *vm) release() {
	v.src = nil
	v.insts = nil
	v.matchData = nil
	if cap(v.stack) <= maxPooledCapacity {
		vmPool.Put(v)
	}
}

func (v *vm) releaseStack() {
	for i := len(v.stack) - 1; i >= 0; i-- {
		v.stack[i].release()
	}
	v.stack = v.stack[:0]
}

func (v *vm) run(programCounter, sourcePos int) (bool, int) {
	backtracks := 0

	for {
		inst := v.insts[programCounter]
		switch inst.op {
		case opChar:
			if sourcePos >= len(v.src) || !matchClass(inst.class, int(v.src[sourcePos])) {
				if !v.backtrack(&programCounter, &sourcePos, &backtracks) {
					return false, sourcePos
				}
				continue
			}
			programCounter++
			sourcePos++

		case opMatch:
			v.releaseStack()
			return true, sourcePos

		case opTailMatch:
			if sourcePos >= len(v.src) {
				v.releaseStack()
				return true, sourcePos
			}
			if !v.backtrack(&programCounter, &sourcePos, &backtracks) {
				return false, sourcePos
			}

		case opJmp:
			programCounter = inst.operand1

		case opSplit:
			t := newVMThread(inst.operand2, sourcePos, v.matchData.captures)
			v.stack = append(v.stack, t)
			programCounter = inst.operand1

		case opSave:
			v.matchData.setCapture(inst.operand1, sourcePos)
			programCounter++

		case opPSave:
			v.matchData.addPosCapture(inst.operand1, sourcePos+1)
			programCounter++

		case opBrace:
			ok, newPos := v.matchBrace(inst.operand1, inst.operand2, sourcePos)
			if !ok {
				if !v.backtrack(&programCounter, &sourcePos, &backtracks) {
					return false, sourcePos
				}
				continue
			}
			sourcePos = newPos
			programCounter++

		case opNumber:
			ok, newPos := v.matchBackref(inst.operand1, sourcePos)
			if !ok {
				if !v.backtrack(&programCounter, &sourcePos, &backtracks) {
					return false, sourcePos
				}
				continue
			}
			programCounter++
			sourcePos = newPos

		default:
			v.releaseStack()
			return false, sourcePos
		}
	}
}

func (v *vm) backtrack(pc, sp, backtracks *int) bool {
	(*backtracks)++
	if *backtracks > MaxBacktracks || len(v.stack) == 0 {
		v.releaseStack()
		return false
	}
	t := v.stack[len(v.stack)-1]
	v.stack = v.stack[:len(v.stack)-1]
	*pc = t.programCounter
	*sp = t.sourcePos
	v.matchData.restoreFrom(t)
	t.release()
	return true
}

func (v *vm) matchBrace(open, close, sourcePos int) (bool, int) {
	if sourcePos >= len(v.src) || int(v.src[sourcePos]) != open {
		return false, sourcePos
	}
	count := 1
	sourcePos++
	for ; sourcePos < len(v.src); sourcePos++ {
		ch := int(v.src[sourcePos])
		switch ch {
		case close:
			count--
			if count == 0 {
				return true, sourcePos + 1
			}
		case open:
			count++
		}
	}
	return false, sourcePos
}

func (v *vm) matchBackref(index, sourcePos int) (bool, int) {
	slot := index * 2
	capLen := v.matchData.CaptureLength()
	if slot+1 >= capLen {
		return false, sourcePos
	}
	start := v.matchData.Capture(slot)
	end := v.matchData.Capture(slot + 1)
	if start > end || end > len(v.src) {
		return false, sourcePos
	}
	capture := v.src[start:end]
	if sourcePos+len(capture) > len(v.src) {
		return false, sourcePos
	}
	if !bytes.Equal(capture, v.src[sourcePos:sourcePos+len(capture)]) {
		return false, sourcePos
	}
	return true, sourcePos + len(capture)
}

// Find searches for pattern matches in src starting at offset.
// Returns up to limit matches (-1 for unlimited).
func Find(pattern string, src []byte, offset, limit int) ([]*MatchData, error) {
	insts, pat, err := getCompiledPattern(pattern)
	if err != nil {
		return nil, err
	}
	capSize := calcMaxCaptureSlot(insts)

	var matches []*MatchData
	if limit > 0 {
		matches = make([]*MatchData, 0, limit)
	}

	v := newVM(src, insts)
	defer v.release()

	for sourcePos := offset; sourcePos <= len(src); {
		md := newMatchData(capSize)
		v.matchData = md
		ok, newPos := v.run(0, sourcePos)
		sourcePos++
		if ok {
			if sourcePos < newPos {
				sourcePos = newPos
			}
			matches = append(matches, md)
			if len(matches) == limit {
				break
			}
		} else {
			md.release()
		}
		if pat.mustHead {
			break
		}
	}
	return matches, nil
}
