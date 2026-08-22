package typ

import "github.com/wippyai/go-lua/types/kind"

type fakeType struct {
	id   string
	hash uint64
}

func (f *fakeType) Kind() kind.Kind { return kind.Record }
func (f *fakeType) String() string  { return f.id }
func (f *fakeType) Hash() uint64    { return f.hash }
func (f *fakeType) Equals(other Type) bool {
	o, ok := other.(*fakeType)
	return ok && f.id == o.id && f.hash == o.hash
}
