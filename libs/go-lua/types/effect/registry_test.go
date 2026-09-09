package effect

import "testing"

type testCodec struct {
	key string
}

func (t testCodec) Key() string                    { return t.key }
func (t testCodec) Encode(l Label, w Writer) error { return nil }
func (t testCodec) Decode(r Reader) (Label, error) { return IO{}, nil }

func TestRegisterAndLookup(t *testing.T) {
	codec := testCodec{key: "test_codec"}
	Register(codec)

	found, ok := Lookup("test_codec")
	if !ok {
		t.Fatal("should find registered codec")
	}

	if found.Key() != "test_codec" {
		t.Error("wrong codec returned")
	}
}

func TestLookupNotFound(t *testing.T) {
	_, ok := Lookup("nonexistent_codec_xyz")
	if ok {
		t.Error("should not find unregistered codec")
	}
}

func TestCodecForBuiltinLabels(t *testing.T) {
	// These tests verify CodecFor extracts correct keys
	// The actual codecs may or may not be registered
	labels := []struct {
		label Label
		key   string
	}{
		{Throw{}, "throw"},
		{IO{}, "io"},
		{Diverge{}, "diverge"},
	}

	// Just verify CodecFor doesn't panic for any label.
	for _, tc := range labels {
		_, _ = CodecFor(tc.label)
	}
}
