package effect

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
)

// mockWriter implements effect.Writer for testing
type mockWriter struct {
	buf   *bytes.Buffer
	err   error
	calls []string
}

func newMockWriter() *mockWriter {
	return &mockWriter{buf: &bytes.Buffer{}}
}

func (w *mockWriter) WriteByte(b byte) error {
	if w.err != nil {
		return w.err
	}

	w.calls = append(w.calls, fmt.Sprintf("WriteByte(%d)", b))

	return w.buf.WriteByte(b)
}

func (w *mockWriter) WriteInt32(v int32) error {
	if w.err != nil {
		return w.err
	}

	w.calls = append(w.calls, fmt.Sprintf("WriteInt32(%d)", v))
	b := make([]byte, 4)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	_, err := w.buf.Write(b)

	return err
}

func (w *mockWriter) WriteString(s string) error {
	if w.err != nil {
		return w.err
	}

	w.calls = append(w.calls, fmt.Sprintf("WriteString(%q)", s))
	if err := w.WriteInt32(int32(len(s))); err != nil {
		return err
	}

	_, err := w.buf.WriteString(s)

	return err
}

func (w *mockWriter) WriteType(t any) error {
	if w.err != nil {
		return w.err
	}

	w.calls = append(w.calls, fmt.Sprintf("WriteType(%v)", t))

	return nil
}

// mockReader implements effect.Reader for testing
type mockReader struct {
	buf *bytes.Buffer
	err error
}

func newMockReader(data []byte) *mockReader {
	return &mockReader{buf: bytes.NewBuffer(data)}
}

func (r *mockReader) ReadByte() (byte, error) {
	if r.err != nil {
		return 0, r.err
	}

	return r.buf.ReadByte()
}

func (r *mockReader) ReadInt32() (int32, error) {
	if r.err != nil {
		return 0, r.err
	}

	b := make([]byte, 4)
	if _, err := r.buf.Read(b); err != nil {
		return 0, err
	}

	return int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16 | int32(b[3])<<24, nil
}

func (r *mockReader) ReadString() (string, error) {
	if r.err != nil {
		return "", r.err
	}

	length, err := r.ReadInt32()
	if err != nil {
		return "", err
	}

	if length == 0 {
		return "", nil
	}

	data := make([]byte, length)
	if _, err := r.buf.Read(data); err != nil {
		return "", err
	}

	return string(data), nil
}

func (r *mockReader) ReadType() (any, error) {
	if r.err != nil {
		return nil, r.err
	}

	return nil, nil
}

func TestThrowCodec_Key(t *testing.T) {
	codec := throwCodec{}
	if got := codec.Key(); got != "throw" {
		t.Errorf("throwCodec.Key() = %q, want %q", got, "throw")
	}
}

func TestThrowCodec_Encode(t *testing.T) {
	codec := throwCodec{}
	w := newMockWriter()
	label := Throw{}

	err := codec.Encode(label, w)
	if err != nil {
		t.Errorf("throwCodec.Encode() error = %v", err)
	}

	if len(w.calls) != 0 {
		t.Errorf("throwCodec.Encode() should not write anything, got %d calls", len(w.calls))
	}
}

func TestThrowCodec_Decode(t *testing.T) {
	codec := throwCodec{}
	r := newMockReader([]byte{})

	label, err := codec.Decode(r)
	if err != nil {
		t.Errorf("throwCodec.Decode() error = %v", err)
	}

	if _, ok := label.(Throw); !ok {
		t.Errorf("throwCodec.Decode() = %T, want Throw", label)
	}
}

func TestThrowCodec_RoundTrip(t *testing.T) {
	codec := throwCodec{}
	original := Throw{}
	w := newMockWriter()

	if err := codec.Encode(original, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())

	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !original.Equals(decoded) {
		t.Errorf("Round trip failed: got %v, want %v", decoded, original)
	}
}

func TestIOCodec_Key(t *testing.T) {
	codec := ioCodec{}
	if got := codec.Key(); got != "io" {
		t.Errorf("ioCodec.Key() = %q, want %q", got, "io")
	}
}

func TestIOCodec_Encode(t *testing.T) {
	codec := ioCodec{}
	w := newMockWriter()
	label := IO{}

	err := codec.Encode(label, w)
	if err != nil {
		t.Errorf("ioCodec.Encode() error = %v", err)
	}
}

func TestIOCodec_Decode(t *testing.T) {
	codec := ioCodec{}
	r := newMockReader([]byte{})

	label, err := codec.Decode(r)
	if err != nil {
		t.Errorf("ioCodec.Decode() error = %v", err)
	}

	if _, ok := label.(IO); !ok {
		t.Errorf("ioCodec.Decode() = %T, want IO", label)
	}
}

func TestIOCodec_RoundTrip(t *testing.T) {
	codec := ioCodec{}
	original := IO{}
	w := newMockWriter()

	if err := codec.Encode(original, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())

	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !original.Equals(decoded) {
		t.Errorf("Round trip failed: got %v, want %v", decoded, original)
	}
}

func TestDivergeCodec_Key(t *testing.T) {
	codec := divergeCodec{}
	if got := codec.Key(); got != "diverge" {
		t.Errorf("divergeCodec.Key() = %q, want %q", got, "diverge")
	}
}

func TestDivergeCodec_RoundTrip(t *testing.T) {
	codec := divergeCodec{}
	original := Diverge{}
	w := newMockWriter()

	if err := codec.Encode(original, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())

	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if !original.Equals(decoded) {
		t.Errorf("Round trip failed: got %v, want %v", decoded, original)
	}
}

func TestMutateCodec_Key(t *testing.T) {
	codec := mutateCodec{}
	if got := codec.Key(); got != "mutate" {
		t.Errorf("mutateCodec.Key() = %q, want %q", got, "mutate")
	}
}

func TestMutateCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		mutate Mutate
	}{
		{
			name: "unchanged transform with nil delta",
			mutate: Mutate{
				Target:      ParamRef{Index: 0},
				Transform:   Unchanged{},
				LengthDelta: nil,
			},
		},
		{
			name: "element union transform",
			mutate: Mutate{
				Target:      ParamRef{Index: 1},
				Transform:   ElementUnion{Source: ParamRef{Index: 2}},
				LengthDelta: nil,
			},
		},
		{
			name: "to array transform",
			mutate: Mutate{
				Target:      ParamRef{Index: 0},
				Transform:   ToArray{Element: ParamRef{Index: 1}},
				LengthDelta: nil,
			},
		},
		{
			name: "with const length delta",
			mutate: Mutate{
				Target:      ParamRef{Index: 0},
				Transform:   Unchanged{},
				LengthDelta: constraint.C(1),
			},
		},
		{
			name: "with var length delta",
			mutate: Mutate{
				Target:      ParamRef{Index: 0},
				Transform:   Unchanged{},
				LengthDelta: constraint.Var{Name: "n"},
			},
		},
		{
			name: "with param length delta",
			mutate: Mutate{
				Target:      ParamRef{Index: 0},
				Transform:   Unchanged{},
				LengthDelta: constraint.PL(1),
			},
		},
	}

	codec := mutateCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.mutate, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			m, ok := decoded.(Mutate)
			if !ok {
				t.Fatalf("Decode() = %T, want Mutate", decoded)
			}

			if m.Target.Index != tt.mutate.Target.Index {
				t.Errorf("Target.Index = %d, want %d", m.Target.Index, tt.mutate.Target.Index)
			}
		})
	}
}

func TestReturnCodec_Key(t *testing.T) {
	codec := returnCodec{}
	if got := codec.Key(); got != "return" {
		t.Errorf("returnCodec.Key() = %q, want %q", got, "return")
	}
}

func TestReturnCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		ret  Return
	}{
		{
			name: "nil transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   nil,
			},
		},
		{
			name: "element of transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   ElementOf{Source: ParamRef{Index: 0}},
			},
		},
		{
			name: "optional element of transform",
			ret: Return{
				ReturnIndex: 1,
				Transform:   OptionalElementOf{Source: ParamRef{Index: 0}},
			},
		},
		{
			name: "callback return transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   CallbackReturn{CallbackParam: ParamRef{Index: 1}},
			},
		},
		{
			name: "array of callback return transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: 1}},
			},
		},
		{
			name: "same as transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   SameAs{Source: ParamRef{Index: 0}},
			},
		},
		{
			name: "deep element of transform",
			ret: Return{
				ReturnIndex: 0,
				Transform:   DeepElementOf{Source: ParamRef{Index: 0}},
			},
		},
	}

	codec := returnCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.ret, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			ret, ok := decoded.(Return)
			if !ok {
				t.Fatalf("Decode() = %T, want Return", decoded)
			}

			if ret.ReturnIndex != tt.ret.ReturnIndex {
				t.Errorf("ReturnIndex = %d, want %d", ret.ReturnIndex, tt.ret.ReturnIndex)
			}
		})
	}
}

func TestErrorReturnCodec_Key(t *testing.T) {
	codec := errorReturnCodec{}
	if got := codec.Key(); got != "error_return" {
		t.Errorf("errorReturnCodec.Key() = %q, want %q", got, "error_return")
	}
}

func TestErrorReturnCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		er   ErrorReturn
	}{
		{
			name: "value0 error1",
			er:   ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		},
		{
			name: "value1 error0",
			er:   ErrorReturn{ValueIndex: 1, ErrorIndex: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			codec := errorReturnCodec{}
			if err := codec.Encode(tt.er, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())
			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			er, ok := decoded.(ErrorReturn)
			if !ok {
				t.Fatalf("Decode() = %T, want ErrorReturn", decoded)
			}
			if er.ValueIndex != tt.er.ValueIndex {
				t.Errorf("ValueIndex = %d, want %d", er.ValueIndex, tt.er.ValueIndex)
			}
			if er.ErrorIndex != tt.er.ErrorIndex {
				t.Errorf("ErrorIndex = %d, want %d", er.ErrorIndex, tt.er.ErrorIndex)
			}
		})
	}
}

func TestReturnLengthCodec_Key(t *testing.T) {
	codec := returnLengthCodec{}
	if got := codec.Key(); got != "return_length" {
		t.Errorf("returnLengthCodec.Key() = %q, want %q", got, "return_length")
	}
}

func TestReturnLengthCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		rl   ReturnLength
	}{
		{
			name: "nil length",
			rl: ReturnLength{
				ReturnIndex: 0,
				Length:      nil,
			},
		},
		{
			name: "const length",
			rl: ReturnLength{
				ReturnIndex: 0,
				Length:      constraint.C(5),
			},
		},
		{
			name: "param length",
			rl: ReturnLength{
				ReturnIndex: 1,
				Length:      constraint.PL(0),
			},
		},
		{
			name: "return length",
			rl: ReturnLength{
				ReturnIndex: 0,
				Length:      constraint.RL(1),
			},
		},
		{
			name: "binary op length",
			rl: ReturnLength{
				ReturnIndex: 0,
				Length:      constraint.BinOp{Op: constraint.OpAdd, Left: constraint.C(1), Right: constraint.C(2)},
			},
		},
	}

	codec := returnLengthCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.rl, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			rl, ok := decoded.(ReturnLength)
			if !ok {
				t.Fatalf("Decode() = %T, want ReturnLength", decoded)
			}

			if rl.ReturnIndex != tt.rl.ReturnIndex {
				t.Errorf("ReturnIndex = %d, want %d", rl.ReturnIndex, tt.rl.ReturnIndex)
			}
		})
	}
}

func TestIteratorCodec_Key(t *testing.T) {
	codec := iteratorCodec{}
	if got := codec.Key(); got != "iterator" {
		t.Errorf("iteratorCodec.Key() = %q, want %q", got, "iterator")
	}
}

func TestIteratorCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		iter Iterator
	}{
		{
			name: "indexed iterator",
			iter: Iterator{
				Source: ParamRef{Index: 0},
				Kind:   IterateIndexed,
			},
		},
		{
			name: "keyed iterator",
			iter: Iterator{
				Source: ParamRef{Index: 1},
				Kind:   IterateKeyed,
			},
		},
	}

	codec := iteratorCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.iter, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			iter, ok := decoded.(Iterator)
			if !ok {
				t.Fatalf("Decode() = %T, want Iterator", decoded)
			}

			if iter.Source.Index != tt.iter.Source.Index {
				t.Errorf("Source.Index = %d, want %d", iter.Source.Index, tt.iter.Source.Index)
			}

			if iter.Kind != tt.iter.Kind {
				t.Errorf("Kind = %v, want %v", iter.Kind, tt.iter.Kind)
			}
		})
	}
}

func TestTableMutatorCodec_Key(t *testing.T) {
	codec := tableMutatorCodec{}
	if got := codec.Key(); got != "table_mutator" {
		t.Errorf("tableMutatorCodec.Key() = %q, want %q", got, "table_mutator")
	}
}

func TestTableMutatorCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		tm   TableMutator
	}{
		{
			name: "basic table mutator",
			tm: TableMutator{
				Target: ParamRef{Index: 0},
				Value:  ParamRef{Index: 1},
			},
		},
		{
			name: "different indices",
			tm: TableMutator{
				Target: ParamRef{Index: 2},
				Value:  ParamRef{Index: 3},
			},
		},
	}

	codec := tableMutatorCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.tm, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			tm, ok := decoded.(TableMutator)
			if !ok {
				t.Fatalf("Decode() = %T, want TableMutator", decoded)
			}

			if tm.Target.Index != tt.tm.Target.Index {
				t.Errorf("Target.Index = %d, want %d", tm.Target.Index, tt.tm.Target.Index)
			}

			if tm.Value.Index != tt.tm.Value.Index {
				t.Errorf("Value.Index = %d, want %d", tm.Value.Index, tt.tm.Value.Index)
			}
		})
	}
}

func TestLengthChangeCodec_Key(t *testing.T) {
	codec := lengthChangeCodec{}
	if got := codec.Key(); got != "length_change" {
		t.Errorf("lengthChangeCodec.Key() = %q, want %q", got, "length_change")
	}
}

func TestBorrowCodec_Key(t *testing.T) {
	codec := borrowCodec{}
	if got := codec.Key(); got != "borrow" {
		t.Errorf("borrowCodec.Key() = %q, want %q", got, "borrow")
	}
}

func TestParamRefCodecs_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		codec     LabelCodec
		value     Label
		getIndex  func(Label) int
		wantIndex int
	}{
		{
			name:      "borrow param index 0",
			codec:     borrowCodec{},
			value:     Borrow{Param: ParamRef{Index: 0}},
			getIndex:  func(l Label) int { return l.(Borrow).Param.Index },
			wantIndex: 0,
		},
		{
			name:      "borrow param index 5",
			codec:     borrowCodec{},
			value:     Borrow{Param: ParamRef{Index: 5}},
			getIndex:  func(l Label) int { return l.(Borrow).Param.Index },
			wantIndex: 5,
		},
		{
			name:      "store param index 0",
			codec:     storeCodec{},
			value:     Store{Param: ParamRef{Index: 0}},
			getIndex:  func(l Label) int { return l.(Store).Param.Index },
			wantIndex: 0,
		},
		{
			name:      "store param index 3",
			codec:     storeCodec{},
			value:     Store{Param: ParamRef{Index: 3}},
			getIndex:  func(l Label) int { return l.(Store).Param.Index },
			wantIndex: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := tt.codec.Encode(tt.value, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := tt.codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			if got := tt.getIndex(decoded); got != tt.wantIndex {
				t.Errorf("Param.Index = %d, want %d", got, tt.wantIndex)
			}
		})
	}
}

func TestStoreCodec_Key(t *testing.T) {
	codec := storeCodec{}
	if got := codec.Key(); got != "store" {
		t.Errorf("storeCodec.Key() = %q, want %q", got, "store")
	}
}

func TestBorrowAllCodec_Key(t *testing.T) {
	codec := borrowAllCodec{}
	if got := codec.Key(); got != "borrow_all" {
		t.Errorf("borrowAllCodec.Key() = %q, want %q", got, "borrow_all")
	}
}

func TestBorrowAllCodec_RoundTrip(t *testing.T) {
	codec := borrowAllCodec{}
	original := BorrowAll{}
	w := newMockWriter()

	if err := codec.Encode(original, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())

	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(BorrowAll); !ok {
		t.Errorf("Decode() = %T, want BorrowAll", decoded)
	}
}

func TestLengthChangeCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		lc   LengthChange
	}{
		{
			name: "positive delta",
			lc: LengthChange{
				Target: ParamRef{Index: 0},
				Delta:  1,
			},
		},
		{
			name: "negative delta",
			lc: LengthChange{
				Target: ParamRef{Index: 1},
				Delta:  -1,
			},
		},
		{
			name: "zero delta",
			lc: LengthChange{
				Target: ParamRef{Index: 0},
				Delta:  0,
			},
		},
	}

	codec := lengthChangeCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.lc, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			lc, ok := decoded.(LengthChange)
			if !ok {
				t.Fatalf("Decode() = %T, want LengthChange", decoded)
			}

			if lc.Target.Index != tt.lc.Target.Index {
				t.Errorf("Target.Index = %d, want %d", lc.Target.Index, tt.lc.Target.Index)
			}

			if lc.Delta != tt.lc.Delta {
				t.Errorf("Delta = %d, want %d", lc.Delta, tt.lc.Delta)
			}
		})
	}
}

func TestWriteReadTransform(t *testing.T) {
	tests := []struct {
		name      string
		transform TypeTransform
	}{
		{
			name:      "nil transform",
			transform: nil,
		},
		{
			name:      "unchanged transform",
			transform: Unchanged{},
		},
		{
			name:      "element union transform",
			transform: ElementUnion{Source: ParamRef{Index: 1}},
		},
		{
			name:      "to array transform",
			transform: ToArray{Element: ParamRef{Index: 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := writeTransform(w, tt.transform); err != nil {
				t.Fatalf("writeTransform() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := readTransform(r)
			if err != nil {
				t.Fatalf("readTransform() error = %v", err)
			}

			if tt.transform == nil {
				if _, ok := decoded.(Unchanged); !ok {
					t.Errorf("nil transform should decode to Unchanged, got %T", decoded)
				}

				return
			}

			if fmt.Sprintf("%T", decoded) != fmt.Sprintf("%T", tt.transform) {
				t.Errorf("Type mismatch: got %T, want %T", decoded, tt.transform)
			}
		})
	}
}

func TestWriteReadReturnType(t *testing.T) {
	tests := []struct {
		name string
		rt   ReturnType
	}{
		{
			name: "nil return type",
			rt:   nil,
		},
		{
			name: "element of",
			rt:   ElementOf{Source: ParamRef{Index: 0}},
		},
		{
			name: "optional element of",
			rt:   OptionalElementOf{Source: ParamRef{Index: 1}},
		},
		{
			name: "callback return",
			rt:   CallbackReturn{CallbackParam: ParamRef{Index: 2}},
		},
		{
			name: "array of callback return",
			rt:   ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: 3}},
		},
		{
			name: "same as",
			rt:   SameAs{Source: ParamRef{Index: 0}},
		},
		{
			name: "deep element of",
			rt:   DeepElementOf{Source: ParamRef{Index: 1}},
		},
		{
			name: "string unpack value",
			rt:   StringUnpackValue{Format: ParamRef{Index: 0}},
		},
		{
			name: "select case of param",
			rt:   SelectCaseOfParam{Source: ParamRef{Index: 1}},
		},
		{
			name: "select result of cases",
			rt:   SelectResultOfCases{Cases: ParamRef{Index: 0}, Default: ParamRef{Index: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := writeReturnType(w, tt.rt); err != nil {
				t.Fatalf("writeReturnType() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := readReturnType(r)
			if err != nil {
				t.Fatalf("readReturnType() error = %v", err)
			}

			if tt.rt == nil {
				if decoded != nil {
					t.Errorf("nil return type should decode to nil, got %T", decoded)
				}

				return
			}

			if fmt.Sprintf("%T", decoded) != fmt.Sprintf("%T", tt.rt) {
				t.Errorf("Type mismatch: got %T, want %T", decoded, tt.rt)
			}
		})
	}
}

func TestWriteReadExpr(t *testing.T) {
	tests := []struct {
		name string
		expr constraint.Expr
	}{
		{
			name: "nil expr",
			expr: nil,
		},
		{
			name: "var expr",
			expr: constraint.Var{Name: "n"},
		},
		{
			name: "const expr",
			expr: constraint.C(42),
		},
		{
			name: "binop expr",
			expr: constraint.BinOp{Op: constraint.OpAdd, Left: constraint.C(1), Right: constraint.C(2)},
		},
		{
			name: "len expr",
			expr: constraint.Len{Of: "param"},
		},
		{
			name: "param expr",
			expr: constraint.Param{Index: 0},
		},
		{
			name: "ret expr",
			expr: constraint.Ret{Index: 1},
		},
		{
			name: "param len expr",
			expr: constraint.PL(0),
		},
		{
			name: "ret len expr",
			expr: constraint.RL(1),
		},
		{
			name: "min expr",
			expr: constraint.Min{Left: constraint.C(1), Right: constraint.C(2)},
		},
		{
			name: "max expr",
			expr: constraint.Max{Left: constraint.C(3), Right: constraint.C(4)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := writeExpr(w, tt.expr); err != nil {
				t.Fatalf("writeExpr() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := readExpr(r)
			if err != nil {
				t.Fatalf("readExpr() error = %v", err)
			}

			if tt.expr == nil {
				if decoded != nil {
					t.Errorf("nil expr should decode to nil, got %T", decoded)
				}

				return
			}

			if fmt.Sprintf("%T", decoded) != fmt.Sprintf("%T", tt.expr) {
				t.Errorf("Type mismatch: got %T, want %T", decoded, tt.expr)
			}
		})
	}
}

func TestRegistryLookup(t *testing.T) {
	tests := []struct {
		key    string
		wantOk bool
	}{
		{KeyThrow, true},
		{KeyIO, true},
		{KeyDiverge, true},
		{KeyMutate, true},
		{KeyReturn, true},
		{KeyReturnLength, true},
		{KeyIterator, true},
		{KeyTableMutator, true},
		{KeyLengthChange, true},
		{KeyBorrow, true},
		{KeyStore, true},
		{KeyBorrowAll, true},
		{KeyCorrelatedReturn, true},
		{KeyModuleLoad, true},
		{KeyVariadicTransform, true},
		{KeyTypePredicate, true},
		{KeyTypeValueMethod, true},
		{KeyCallableType, true},
		{"nonexistent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			codec, ok := Lookup(tt.key)
			if ok != tt.wantOk {
				t.Errorf("Lookup(%q) ok = %v, want %v", tt.key, ok, tt.wantOk)
			}

			if ok && codec == nil {
				t.Errorf("Lookup(%q) returned nil codec", tt.key)
			}

			if ok && codec.Key() != tt.key {
				t.Errorf("Lookup(%q).Key() = %q, want %q", tt.key, codec.Key(), tt.key)
			}
		})
	}
}

func TestCodecFor(t *testing.T) {
	tests := []struct {
		name    string
		label   Label
		wantKey string
		wantOk  bool
	}{
		{
			name:    "throw",
			label:   Throw{},
			wantKey: "throw",
			wantOk:  true,
		},
		{
			name:    "io",
			label:   IO{},
			wantKey: "io",
			wantOk:  true,
		},
		{
			name:    "diverge",
			label:   Diverge{},
			wantKey: "diverge",
			wantOk:  true,
		},
		{
			name:    "mutate",
			label:   Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}},
			wantKey: "mutate",
			wantOk:  true,
		},
		{
			name:    "return",
			label:   Return{ReturnIndex: 0, Transform: nil},
			wantKey: "return",
			wantOk:  true,
		},
		{
			name:    "error return",
			label:   ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
			wantKey: "error_return",
			wantOk:  true,
		},
		{
			name:    "return length",
			label:   ReturnLength{ReturnIndex: 0, Length: constraint.C(1)},
			wantKey: "return_length",
			wantOk:  true,
		},
		{
			name:    "iterator",
			label:   Iterator{Source: ParamRef{Index: 0}, Kind: IterateIndexed},
			wantKey: "iterator",
			wantOk:  true,
		},
		{
			name:    "table mutator",
			label:   TableMutator{Target: ParamRef{Index: 0}, Value: ParamRef{Index: 1}},
			wantKey: "table_mutator",
			wantOk:  true,
		},
		{
			name:    "length change",
			label:   LengthChange{Target: ParamRef{Index: 0}, Delta: 1},
			wantKey: "length_change",
			wantOk:  true,
		},
		{
			name:    "borrow",
			label:   Borrow{Param: ParamRef{Index: 0}},
			wantKey: "borrow",
			wantOk:  true,
		},
		{
			name:    "store",
			label:   Store{Param: ParamRef{Index: 1}},
			wantKey: "store",
			wantOk:  true,
		},
		{
			name:    "borrow all",
			label:   BorrowAll{},
			wantKey: "borrow_all",
			wantOk:  true,
		},
		{
			name:    "correlated return",
			label:   CorrelatedReturn{Indices: []int{0, 1}},
			wantKey: "correlated_return",
			wantOk:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec, ok := CodecFor(tt.label)
			if ok != tt.wantOk {
				t.Errorf("CodecFor(%v) ok = %v, want %v", tt.label, ok, tt.wantOk)
			}

			if ok && codec.Key() != tt.wantKey {
				t.Errorf("CodecFor(%v).Key() = %q, want %q", tt.label, codec.Key(), tt.wantKey)
			}
		})
	}
}

func TestWriteReadTransform_InvalidTag(t *testing.T) {
	w := newMockWriter()
	_ = w.WriteByte(99)

	r := newMockReader(w.buf.Bytes())

	decoded, err := readTransform(r)
	if err != nil {
		t.Errorf("readTransform() with invalid tag should not error, got %v", err)
	}

	if _, ok := decoded.(Unchanged); !ok {
		t.Errorf("readTransform() with invalid tag should return Unchanged, got %T", decoded)
	}
}

func TestWriteReadReturnType_InvalidTag(t *testing.T) {
	w := newMockWriter()
	_ = w.WriteByte(99)

	r := newMockReader(w.buf.Bytes())

	decoded, err := readReturnType(r)
	if err == nil {
		t.Error("readReturnType() with invalid tag should return error")
	}

	if decoded != nil {
		t.Errorf("readReturnType() with invalid tag should return nil, got %T", decoded)
	}
}

func TestWriteReadExpr_InvalidTag(t *testing.T) {
	w := newMockWriter()
	_ = w.WriteByte(99)

	r := newMockReader(w.buf.Bytes())

	decoded, err := readExpr(r)
	if err == nil {
		t.Error("readExpr() with invalid tag should return error")
	}

	if decoded != nil {
		t.Errorf("readExpr() with invalid tag should return nil, got %T", decoded)
	}
}

func TestWriteReadExpr_NestedBinOp(t *testing.T) {
	expr := constraint.BinOp{
		Op: constraint.OpAdd,
		Left: constraint.BinOp{
			Op:    constraint.OpMul,
			Left:  constraint.C(2),
			Right: constraint.C(3),
		},
		Right: constraint.C(1),
	}

	w := newMockWriter()
	if err := writeExpr(w, expr); err != nil {
		t.Fatalf("writeExpr() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())

	decoded, err := readExpr(r)
	if err != nil {
		t.Fatalf("readExpr() error = %v", err)
	}

	binop, ok := decoded.(constraint.BinOp)
	if !ok {
		t.Fatalf("decoded type = %T, want BinOp", decoded)
	}

	if binop.Op != constraint.OpAdd {
		t.Errorf("Op = %v, want %v", binop.Op, constraint.OpAdd)
	}
}

func TestAllCodecsRegistered(t *testing.T) {
	expectedKeys := []string{
		"throw",
		"io",
		"diverge",
		"mutate",
		"return",
		"return_length",
		"iterator",
		"table_mutator",
		"length_change",
		"borrow",
		"store",
		"borrow_all",
		"correlated_return",
	}

	for _, key := range expectedKeys {
		if _, ok := Lookup(key); !ok {
			t.Errorf("Expected codec %q to be registered", key)
		}
	}
}

func TestWriteReadExpr_AllTypes(t *testing.T) {
	tests := []struct {
		name string
		expr constraint.Expr
	}{
		{
			name: "binop subtract",
			expr: constraint.BinOp{Op: constraint.OpSub, Left: constraint.C(5), Right: constraint.C(3)},
		},
		{
			name: "binop multiply",
			expr: constraint.BinOp{Op: constraint.OpMul, Left: constraint.C(2), Right: constraint.C(4)},
		},
		{
			name: "binop divide",
			expr: constraint.BinOp{Op: constraint.OpDiv, Left: constraint.C(8), Right: constraint.C(2)},
		},
		{
			name: "nested min",
			expr: constraint.Min{
				Left:  constraint.Min{Left: constraint.C(1), Right: constraint.C(2)},
				Right: constraint.C(3),
			},
		},
		{
			name: "nested max",
			expr: constraint.Max{
				Left:  constraint.Max{Left: constraint.C(10), Right: constraint.C(20)},
				Right: constraint.C(15),
			},
		},
		{
			name: "complex nested binop",
			expr: constraint.BinOp{
				Op: constraint.OpAdd,
				Left: constraint.BinOp{
					Op:    constraint.OpMul,
					Left:  constraint.PL(0),
					Right: constraint.C(2),
				},
				Right: constraint.RL(1),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := writeExpr(w, tt.expr); err != nil {
				t.Fatalf("writeExpr() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := readExpr(r)
			if err != nil {
				t.Fatalf("readExpr() error = %v", err)
			}

			if decoded == nil {
				t.Fatalf("decoded expr is nil")
			}
		})
	}
}

func TestMutateCodec_ErrorHandling(t *testing.T) {
	codec := mutateCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestReturnCodec_ErrorHandling(t *testing.T) {
	codec := returnCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestReturnLengthCodec_ErrorHandling(t *testing.T) {
	codec := returnLengthCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestIteratorCodec_ErrorHandling(t *testing.T) {
	codec := iteratorCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestTableMutatorCodec_ErrorHandling(t *testing.T) {
	codec := tableMutatorCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestLengthChangeCodec_ErrorHandling(t *testing.T) {
	codec := lengthChangeCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestBorrowCodec_ErrorHandling(t *testing.T) {
	codec := borrowCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestStoreCodec_ErrorHandling(t *testing.T) {
	codec := storeCodec{}

	t.Run("decode with insufficient data", func(t *testing.T) {
		r := newMockReader([]byte{})

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("Expected error when reading from empty buffer")
		}
	})
}

func TestReadTransform_ErrorHandling(t *testing.T) {
	t.Run("insufficient data for element union", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(transformElementUnion)

		r := newMockReader(w.buf.Bytes())

		_, err := readTransform(r)
		if err == nil {
			t.Error("Expected error when reading incomplete ElementUnion")
		}
	})

	t.Run("insufficient data for to array", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(transformToArray)

		r := newMockReader(w.buf.Bytes())

		_, err := readTransform(r)
		if err == nil {
			t.Error("Expected error when reading incomplete ToArray")
		}
	})
}

func TestReadReturnType_ErrorHandling(t *testing.T) {
	t.Run("insufficient data for element of", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeElementOf)

		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("Expected error when reading incomplete ElementOf")
		}
	})

	t.Run("insufficient data for callback return", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeCallbackReturn)

		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("Expected error when reading incomplete CallbackReturn")
		}
	})
}

func TestReadExpr_ErrorHandling(t *testing.T) {
	tests := []struct {
		name  string
		setup func(w *mockWriter)
	}{
		{"insufficient data for var", func(w *mockWriter) { _ = w.WriteByte(exprVar) }},
		{"insufficient data for const", func(w *mockWriter) { _ = w.WriteByte(exprConst) }},
		{"insufficient data for binop", func(w *mockWriter) { _ = w.WriteByte(exprBinOp) }},
		{"insufficient data for len", func(w *mockWriter) { _ = w.WriteByte(exprLen) }},
		{"param error", func(w *mockWriter) { _ = w.WriteByte(exprParam) }},
		{"ret error", func(w *mockWriter) { _ = w.WriteByte(exprRet) }},
		{"param len error", func(w *mockWriter) { _ = w.WriteByte(exprParamLen) }},
		{"ret len error", func(w *mockWriter) { _ = w.WriteByte(exprRetLen) }},
		{"min left error", func(w *mockWriter) { _ = w.WriteByte(exprMin) }},
		{"min right error", func(w *mockWriter) { _ = w.WriteByte(exprMin); _ = w.WriteByte(exprNil) }},
		{"max left error", func(w *mockWriter) { _ = w.WriteByte(exprMax) }},
		{"max right error", func(w *mockWriter) { _ = w.WriteByte(exprMax); _ = w.WriteByte(exprNil) }},
		{"binop left error", func(w *mockWriter) {
			_ = w.WriteByte(exprBinOp)
			_ = w.WriteByte(byte(constraint.OpAdd))
		}},
		{"binop right error", func(w *mockWriter) {
			_ = w.WriteByte(exprBinOp)
			_ = w.WriteByte(byte(constraint.OpAdd))
			_ = w.WriteByte(exprNil)
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := newMockWriter()
			tc.setup(w)
			r := newMockReader(w.buf.Bytes())

			_, err := readExpr(r)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestMutateCodec_ComplexExpressions(t *testing.T) {
	codec := mutateCodec{}

	tests := []struct {
		name   string
		mutate Mutate
	}{
		{
			name: "binop with nested expressions",
			mutate: Mutate{
				Target:    ParamRef{Index: 0},
				Transform: ElementUnion{Source: ParamRef{Index: 1}},
				LengthDelta: constraint.BinOp{
					Op: constraint.OpAdd,
					Left: constraint.BinOp{
						Op:    constraint.OpMul,
						Left:  constraint.C(2),
						Right: constraint.PL(1),
					},
					Right: constraint.C(1),
				},
			},
		},
		{
			name: "min max expressions",
			mutate: Mutate{
				Target:    ParamRef{Index: 2},
				Transform: ToArray{Element: ParamRef{Index: 0}},
				LengthDelta: constraint.Min{
					Left:  constraint.Max{Left: constraint.C(1), Right: constraint.C(10)},
					Right: constraint.PL(0),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.mutate, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			m, ok := decoded.(Mutate)
			if !ok {
				t.Fatalf("Decode() = %T, want Mutate", decoded)
			}

			if m.Target.Index != tt.mutate.Target.Index {
				t.Errorf("Target.Index = %d, want %d", m.Target.Index, tt.mutate.Target.Index)
			}
		})
	}
}

func TestEncodeErrorPaths(t *testing.T) {
	errTest := errors.New("test error")

	t.Run("mutate encode target error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := mutateCodec{}

		err := codec.Encode(Mutate{Target: ParamRef{Index: 0}}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})

	t.Run("mutate encode transform error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 1}
		codec := mutateCodec{}

		err := codec.Encode(Mutate{Target: ParamRef{Index: 0}, Transform: Unchanged{}}, w)
		if err == nil {
			t.Error("expected error on transform write")
		}
	})

	t.Run("return encode index error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := returnCodec{}

		err := codec.Encode(Return{ReturnIndex: 0}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})

	t.Run("return length encode index error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := returnLengthCodec{}

		err := codec.Encode(ReturnLength{ReturnIndex: 0}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})

	t.Run("iterator encode source error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := iteratorCodec{}

		err := codec.Encode(Iterator{Source: ParamRef{Index: 0}}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})

	t.Run("table mutator encode target error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := tableMutatorCodec{}

		err := codec.Encode(TableMutator{Target: ParamRef{Index: 0}}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})

	t.Run("length change encode target error", func(t *testing.T) {
		w := newMockWriter()
		w.err = errTest
		codec := lengthChangeCodec{}

		err := codec.Encode(LengthChange{Target: ParamRef{Index: 0}}, w)
		if err != errTest {
			t.Errorf("expected error %v, got %v", errTest, err)
		}
	})
}

type errorAfterNWriter struct {
	n     int
	count int
}

func (w *errorAfterNWriter) WriteByte(b byte) error {
	w.count++
	if w.count > w.n {
		return fmt.Errorf("error after %d writes", w.n)
	}

	return nil
}

func (w *errorAfterNWriter) WriteInt32(v int32) error {
	w.count++
	if w.count > w.n {
		return fmt.Errorf("error after %d writes", w.n)
	}

	return nil
}

func (w *errorAfterNWriter) WriteString(s string) error {
	w.count++
	if w.count > w.n {
		return fmt.Errorf("error after %d writes", w.n)
	}

	return nil
}

func (w *errorAfterNWriter) WriteType(t any) error {
	w.count++
	if w.count > w.n {
		return fmt.Errorf("error after %d writes", w.n)
	}

	return nil
}

func TestWriteTransformErrorPaths(t *testing.T) {
	t.Run("element union tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeTransform(w, ElementUnion{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("to array tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeTransform(w, ToArray{Element: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestWriteReturnTypeErrorPaths(t *testing.T) {
	t.Run("element of tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, ElementOf{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("optional element of tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, OptionalElementOf{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("callback return tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, CallbackReturn{CallbackParam: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("array of callback tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("same as tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, SameAs{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("deep element tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, DeepElementOf{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("string unpack value tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, StringUnpackValue{Format: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("select case of param tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, SelectCaseOfParam{Source: ParamRef{Index: 0}})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("select result of cases tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeReturnType(w, SelectResultOfCases{Cases: ParamRef{Index: 0}, Default: ParamRef{Index: 1}})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestWriteExprErrorPaths(t *testing.T) {
	t.Run("var tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Var{Name: "x"})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("const tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.C(1))
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("binop tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.BinOp{Op: constraint.OpAdd, Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("binop left error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 2}

		err := writeExpr(w, constraint.BinOp{Op: constraint.OpAdd, Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error on left")
		}
	})

	t.Run("len tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Len{Of: "x"})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("param tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Param{Index: 0})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("ret tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Ret{Index: 0})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("param len tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.PL(0))
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("ret len tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.RL(0))
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("min tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Min{Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("min left error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 1}

		err := writeExpr(w, constraint.Min{Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("max tag error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 0}

		err := writeExpr(w, constraint.Max{Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("max left error", func(t *testing.T) {
		w := &errorAfterNWriter{n: 1}

		err := writeExpr(w, constraint.Max{Left: constraint.C(1), Right: constraint.C(2)})
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestReadReturnTypeMoreErrors(t *testing.T) {
	t.Run("optional element of error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeOptionalElementOf)
		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("array of callback error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeArrayOfCallback)
		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("same as error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeSameAs)
		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("deep element error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteByte(returnTypeDeepElementOf)
		r := newMockReader(w.buf.Bytes())

		_, err := readReturnType(r)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestDecodePartialErrors(t *testing.T) {
	t.Run("iterator kind error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := iteratorCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing kind")
		}
	})

	t.Run("table mutator value error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := tableMutatorCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing value")
		}
	})

	t.Run("length change delta error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := lengthChangeCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing delta")
		}
	})

	t.Run("mutate transform error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := mutateCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing transform")
		}
	})

	t.Run("mutate delta error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		_ = w.WriteByte(transformUnchanged)
		r := newMockReader(w.buf.Bytes())
		codec := mutateCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing delta")
		}
	})

	t.Run("return transform error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := returnCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing transform")
		}
	})

	t.Run("return length expr error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := returnLengthCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing length")
		}
	})
}

type unknownTransform struct{}

func (unknownTransform) String() string { return "unknown" }
func (unknownTransform) transform()     {}

type unknownReturnType struct{}

func (unknownReturnType) String() string { return "unknown" }
func (unknownReturnType) returnType()    {}

func TestWriteUnknownTypes(t *testing.T) {
	t.Run("unknown transform", func(t *testing.T) {
		w := newMockWriter()

		err := writeTransform(w, unknownTransform{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		r := newMockReader(w.buf.Bytes())

		decoded, err := readTransform(r)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if _, ok := decoded.(Unchanged); !ok {
			t.Errorf("expected Unchanged for unknown transform, got %T", decoded)
		}
	})

	t.Run("unknown return type", func(t *testing.T) {
		w := newMockWriter()

		err := writeReturnType(w, unknownReturnType{})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		r := newMockReader(w.buf.Bytes())

		decoded, err := readReturnType(r)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if decoded != nil {
			t.Errorf("expected nil for unknown return type, got %T", decoded)
		}
	})
}

func TestPassThroughCodec_Key(t *testing.T) {
	codec := passThroughCodec{}
	if got := codec.Key(); got != "passthrough" {
		t.Errorf("passThroughCodec.Key() = %q, want %q", got, "passthrough")
	}
}

func TestPassThroughCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		pt   PassThrough
	}{
		{"param0 to ret0", PassThrough{ParamIndex: 0, ReturnIndex: 0}},
		{"param1 to ret0", PassThrough{ParamIndex: 1, ReturnIndex: 0}},
		{"param0 to ret1", PassThrough{ParamIndex: 0, ReturnIndex: 1}},
		{"param2 to ret3", PassThrough{ParamIndex: 2, ReturnIndex: 3}},
	}

	codec := passThroughCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.pt, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			pt, ok := decoded.(PassThrough)
			if !ok {
				t.Fatalf("Decode() = %T, want PassThrough", decoded)
			}

			if pt.ParamIndex != tt.pt.ParamIndex {
				t.Errorf("ParamIndex = %d, want %d", pt.ParamIndex, tt.pt.ParamIndex)
			}

			if pt.ReturnIndex != tt.pt.ReturnIndex {
				t.Errorf("ReturnIndex = %d, want %d", pt.ReturnIndex, tt.pt.ReturnIndex)
			}
		})
	}
}

func TestFlowIntoCodec_Key(t *testing.T) {
	codec := flowIntoCodec{}
	if got := codec.Key(); got != "flowinto" {
		t.Errorf("flowIntoCodec.Key() = %q, want %q", got, "flowinto")
	}
}

func TestFlowIntoCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		fi   FlowInto
	}{
		{"simple path", FlowInto{ParamIndex: 0, ReturnIndex: 0, Path: "inner"}},
		{"nested path", FlowInto{ParamIndex: 1, ReturnIndex: 0, Path: "data.value"}},
		{"empty path", FlowInto{ParamIndex: 0, ReturnIndex: 1, Path: ""}},
	}

	codec := flowIntoCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.fi, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			fi, ok := decoded.(FlowInto)
			if !ok {
				t.Fatalf("Decode() = %T, want FlowInto", decoded)
			}

			if fi.ParamIndex != tt.fi.ParamIndex {
				t.Errorf("ParamIndex = %d, want %d", fi.ParamIndex, tt.fi.ParamIndex)
			}

			if fi.ReturnIndex != tt.fi.ReturnIndex {
				t.Errorf("ReturnIndex = %d, want %d", fi.ReturnIndex, tt.fi.ReturnIndex)
			}

			if fi.Path != tt.fi.Path {
				t.Errorf("Path = %q, want %q", fi.Path, tt.fi.Path)
			}
		})
	}
}

func TestSendCodec_Key(t *testing.T) {
	codec := sendCodec{}
	if got := codec.Key(); got != "send" {
		t.Errorf("sendCodec.Key() = %q, want %q", got, "send")
	}
}

func TestSendCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		send Send
	}{
		{"from param 0", Send{FromParam: 0}},
		{"from param 2", Send{FromParam: 2}},
		{"from param 5", Send{FromParam: 5}},
	}

	codec := sendCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.send, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			s, ok := decoded.(Send)
			if !ok {
				t.Fatalf("Decode() = %T, want Send", decoded)
			}

			if s.FromParam != tt.send.FromParam {
				t.Errorf("FromParam = %d, want %d", s.FromParam, tt.send.FromParam)
			}
		})
	}
}

func TestFreezeCodec_Key(t *testing.T) {
	codec := freezeCodec{}
	if got := codec.Key(); got != "freeze" {
		t.Errorf("freezeCodec.Key() = %q, want %q", got, "freeze")
	}
}

func TestFreezeCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		freeze Freeze
	}{
		{"param 0", Freeze{Param: ParamRef{Index: 0}}},
		{"param 1", Freeze{Param: ParamRef{Index: 1}}},
		{"param 5", Freeze{Param: ParamRef{Index: 5}}},
	}

	codec := freezeCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.freeze, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			f, ok := decoded.(Freeze)
			if !ok {
				t.Fatalf("Decode() = %T, want Freeze", decoded)
			}

			if f.Param.Index != tt.freeze.Param.Index {
				t.Errorf("Param.Index = %d, want %d", f.Param.Index, tt.freeze.Param.Index)
			}
		})
	}
}

func TestCorrelatedReturnCodec_Key(t *testing.T) {
	codec := correlatedReturnCodec{}
	if got := codec.Key(); got != "correlated_return" {
		t.Errorf("correlatedReturnCodec.Key() = %q, want %q", got, "correlated_return")
	}
}

func TestCorrelatedReturnCodec_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		cr   CorrelatedReturn
	}{
		{"two indices", CorrelatedReturn{Indices: []int{0, 1}}},
		{"three indices", CorrelatedReturn{Indices: []int{0, 1, 2}}},
		{"non-zero start", CorrelatedReturn{Indices: []int{1, 3}}},
	}

	codec := correlatedReturnCodec{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newMockWriter()
			if err := codec.Encode(tt.cr, w); err != nil {
				t.Fatalf("Encode() error = %v", err)
			}

			r := newMockReader(w.buf.Bytes())

			decoded, err := codec.Decode(r)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			cr, ok := decoded.(CorrelatedReturn)
			if !ok {
				t.Fatalf("Decode() = %T, want CorrelatedReturn", decoded)
			}

			if len(cr.Indices) != len(tt.cr.Indices) {
				t.Fatalf("Indices length = %d, want %d", len(cr.Indices), len(tt.cr.Indices))
			}

			for i := range cr.Indices {
				if cr.Indices[i] != tt.cr.Indices[i] {
					t.Errorf("Indices[%d] = %d, want %d", i, cr.Indices[i], tt.cr.Indices[i])
				}
			}
		})
	}
}

func TestCorrelatedReturnCodec_ErrorHandling(t *testing.T) {
	t.Run("decode empty", func(t *testing.T) {
		r := newMockReader([]byte{})
		codec := correlatedReturnCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestNewCodecsRegistered(t *testing.T) {
	newKeys := []string{"passthrough", "flowinto", "send", "freeze"}
	for _, key := range newKeys {
		if _, ok := Lookup(key); !ok {
			t.Errorf("Expected codec %q to be registered", key)
		}
	}
}

func TestModuleLoadCodec_RoundTrip(t *testing.T) {
	codec := moduleLoadCodec{}
	if got := codec.Key(); got != KeyModuleLoad {
		t.Errorf("moduleLoadCodec.Key() = %q, want %q", got, KeyModuleLoad)
	}

	w := newMockWriter()
	if err := codec.Encode(ModuleLoad{}, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())
	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(ModuleLoad); !ok {
		t.Errorf("Decode() = %T, want ModuleLoad", decoded)
	}
}

func TestVariadicTransformCodec_RoundTrip(t *testing.T) {
	codec := variadicTransformCodec{}
	if got := codec.Key(); got != KeyVariadicTransform {
		t.Errorf("variadicTransformCodec.Key() = %q, want %q", got, KeyVariadicTransform)
	}

	w := newMockWriter()
	if err := codec.Encode(VariadicTransform{}, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())
	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(VariadicTransform); !ok {
		t.Errorf("Decode() = %T, want VariadicTransform", decoded)
	}
}

func TestTypePredicateCodec_RoundTrip(t *testing.T) {
	codec := typePredicateCodec{}
	if got := codec.Key(); got != KeyTypePredicate {
		t.Errorf("typePredicateCodec.Key() = %q, want %q", got, KeyTypePredicate)
	}

	w := newMockWriter()
	if err := codec.Encode(TypePredicate{}, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())
	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(TypePredicate); !ok {
		t.Errorf("Decode() = %T, want TypePredicate", decoded)
	}
}

func TestTypeValueMethodCodec_RoundTrip(t *testing.T) {
	codec := typeValueMethodCodec{}
	if got := codec.Key(); got != KeyTypeValueMethod {
		t.Errorf("typeValueMethodCodec.Key() = %q, want %q", got, KeyTypeValueMethod)
	}

	w := newMockWriter()
	if err := codec.Encode(TypeValueMethod{}, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())
	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(TypeValueMethod); !ok {
		t.Errorf("Decode() = %T, want TypeValueMethod", decoded)
	}
}

func TestCallableTypeCodec_RoundTrip(t *testing.T) {
	codec := callableTypeCodec{}
	if got := codec.Key(); got != KeyCallableType {
		t.Errorf("callableTypeCodec.Key() = %q, want %q", got, KeyCallableType)
	}

	w := newMockWriter()
	if err := codec.Encode(CallableType{}, w); err != nil {
		t.Fatalf("Encode() error = %v", err)
	}

	r := newMockReader(w.buf.Bytes())
	decoded, err := codec.Decode(r)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	if _, ok := decoded.(CallableType); !ok {
		t.Errorf("Decode() = %T, want CallableType", decoded)
	}
}

func TestNewCodecsErrorHandling(t *testing.T) {
	t.Run("passthrough decode error", func(t *testing.T) {
		r := newMockReader([]byte{})
		codec := passThroughCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("passthrough decode partial error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := passThroughCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing return index")
		}
	})

	t.Run("flowinto decode error", func(t *testing.T) {
		r := newMockReader([]byte{})
		codec := flowIntoCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("flowinto decode partial error", func(t *testing.T) {
		w := newMockWriter()
		_ = w.WriteInt32(0)
		_ = w.WriteInt32(0)
		r := newMockReader(w.buf.Bytes())
		codec := flowIntoCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("send decode error", func(t *testing.T) {
		r := newMockReader([]byte{})
		codec := sendCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("freeze decode error", func(t *testing.T) {
		r := newMockReader([]byte{})
		codec := freezeCodec{}

		_, err := codec.Decode(r)
		if err == nil {
			t.Error("expected error")
		}
	})
}
