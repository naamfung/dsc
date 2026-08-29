package marshaler

import (
	"github.com/jig/lisp/types"
)

type HashMap interface {
	MarshalHashMap() (types.MalType, error)
}

type FactoryJSON interface {
	FromJSON(b []byte) (types.MalType, error)
}

type FactoryHashMap interface {
	FromHashMap(data types.MalType) (types.MalType, error)
}
