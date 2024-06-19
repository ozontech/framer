package encoding

import (
	kv "github.com/ozontech/framer/formats/internal/kv"
	jsonkv "github.com/ozontech/framer/formats/internal/kv/json"
	"github.com/ozontech/framer/formats/model"
)

type Encoder struct {
	metaMarshaler kv.Marshaler
}

func NewEncoder() *Encoder {
	return &Encoder{jsonkv.NewMultiVal()}
}

func (encoder *Encoder) MarshalAppend(b []byte, d *model.Data) ([]byte, error) {
	b = append(b, d.Tag...)
	b = append(b, '\n')

	b = append(b, d.Method...)
	b = append(b, '\n')

	b = encoder.metaMarshaler.MarshalAppend(b, d.Metadata)
	b = append(b, '\n')

	b = append(b, d.Message...)

	return b, nil
}

var _ model.Marshaler = &Encoder{}
