package kv

import (
	"github.com/ozontech/framer/formats/model"
)

type Marshaler interface {
	MarshalAppend(b []byte, meta []model.Meta) []byte
}

type Unmarshaler interface {
	UnmarshalAppend(buf []model.Meta, bytes []byte) ([]model.Meta, error)
}
