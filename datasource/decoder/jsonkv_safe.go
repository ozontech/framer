package decoder

import (
	"github.com/mailru/easyjson/jlexer"
	"github.com/ozontech/framer/utils/lru"
)

const (
	kLRUSize = 1 << 10
	vLRUSize = 1 << 16
)

type SafeMultiValDecoder struct {
	kLRU *lru.LRU
	vLRU *lru.LRU
}

func NewSafeMultiValDecoder() *SafeMultiValDecoder {
	return &SafeMultiValDecoder{
		lru.New(kLRUSize),
		lru.New(vLRUSize),
	}
}

func (d *SafeMultiValDecoder) UnmarshalAppend(buf []Meta, bytes []byte) ([]Meta, error) {
	in := jlexer.Lexer{Data: bytes}

	in.Delim('{')
	for !in.IsDelim('}') {
		key := d.kLRU.GetOrAdd(in.UnsafeBytes())
		in.WantColon()

		in.Delim('[')
		for !in.IsDelim(']') {
			val := d.kLRU.GetOrAdd(in.UnsafeBytes())
			buf = append(buf, Meta{Name: key, Value: val})
			in.WantComma()
		}
		in.Delim(']')

		in.WantComma()
	}
	in.Delim('}')
	in.Consumed()

	return buf, in.Error()
}
