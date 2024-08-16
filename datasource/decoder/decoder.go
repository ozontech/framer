package decoder

import (
	"bytes"
	"fmt"

	"github.com/ozontech/framer/utils/lru"
)

type Decoder struct {
	metaUnmarshaler *SafeMultiValDecoder

	tagsLRU    *lru.LRU
	methodsLRU *lru.LRU
}

func NewDecoder() *Decoder {
	return &Decoder{
		metaUnmarshaler: NewSafeMultiValDecoder(),

		tagsLRU:    lru.New(1024),
		methodsLRU: lru.New(1024),
	}
}

func (decoder *Decoder) Unmarshal(d *Data, b []byte) error {
	d.Reset()

	tagB, b := nextLine(b)
	d.Tag = decoder.tagsLRU.GetOrAdd(tagB)

	methodB, b := nextLine(b)
	d.Method = decoder.methodsLRU.GetOrAdd(methodB)

	metaBytes, b := nextLine(b)
	var err error
	d.Metadata, err = decoder.metaUnmarshaler.UnmarshalAppend(d.Metadata, metaBytes)
	if err != nil {
		return fmt.Errorf("meta unmarshal error: %w", err)
	}

	d.Message = b
	return nil
}

func nextLine(in []byte) ([]byte, []byte) {
	index := bytes.IndexByte(in, '\n')
	if index == -1 {
		return []byte{}, []byte{}
	}
	return in[:index], in[index+1:]
}
