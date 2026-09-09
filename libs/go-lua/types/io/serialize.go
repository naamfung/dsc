// Package io provides binary serialization and deserialization for Lua types.
//
// The package supports full round-trip encoding of all type system constructs
// including primitives, composites (records, functions, unions), generics,
// and flow-sensitive constraints. Serialized types are stored in manifests
// for cross-module type resolution.
//
// Primary APIs:
//
//   - Encode/Decode: Serialize individual types to/from bytes
//   - Manifest: Cross-module type information container
//   - FunctionSummary: Interprocedural analysis data
//
// The binary format uses little-endian encoding with length-prefixed strings
// and type-tagged values. Version numbers track format changes for backward
// compatibility checking.
//
// Usage:
//
//	data, err := io.Encode(typ.String)
//	decoded, err := io.Decode(data)
//
//	manifest := io.NewManifest("mymodule")
//	manifest.SetExport(exportType)
//	data, err := manifest.Encode()
package io

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"slices"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

var (
	ErrInvalidType   = errors.New("invalid type encoding")
	ErrUnknownType   = errors.New("unknown type kind")
	ErrCorruptedData = errors.New("corrupted type data")
)

const (
	annotationArgNil byte = iota
	annotationArgString
	annotationArgInt
	annotationArgFloat
	annotationArgBool
)

func Encode(t typ.Type) ([]byte, error) {
	var buf bytes.Buffer
	w := &typeWriter{w: &buf}
	w.writeType(t)

	if w.err != nil {
		return nil, w.err
	}

	return buf.Bytes(), nil
}

func Decode(data []byte) (typ.Type, error) {
	r := &typeReader{r: bytes.NewReader(data)}
	t := r.readType()

	if r.err != nil {
		return nil, r.err
	}

	return t, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	slices.Sort(keys)

	return keys
}

func binaryWrite(w io.Writer, v uint32) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func binaryWrite64(w io.Writer, v uint64) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func binaryRead(r io.Reader, v *uint32) error {
	return binary.Read(r, binary.LittleEndian, v)
}

func binaryRead64(r io.Reader, v *uint64) error {
	return binary.Read(r, binary.LittleEndian, v)
}

func kindToByte(k kind.Kind) byte {
	return byte(k)
}

func byteToKind(b byte) kind.Kind {
	return kind.Kind(b)
}
