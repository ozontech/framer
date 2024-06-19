package encoding

import (
	"bytes"
	"fmt"

	kv "github.com/ozontech/framer/formats/internal/kv"
	jsonkv "github.com/ozontech/framer/formats/internal/kv/json"
	"github.com/ozontech/framer/formats/model"
)

type Decoder struct {
	metaUnmarshaler kv.Unmarshaler
}

func NewDecoder() *Decoder {
	return &Decoder{
		metaUnmarshaler: jsonkv.NewMultiVal(),
	}
}

func (decoder *Decoder) Unmarshal(d *model.Data, b []byte) error {
	d.Reset()

	d.Tag, b = nextLine(b)
	d.Method, b = nextLine(b)

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

var _ model.Unmarshaler = &Decoder{}
