package encoding

import (
	"bytes"
	"fmt"

	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	"github.com/ozontech/framer/formats/internal/json"
	"github.com/ozontech/framer/formats/internal/kv"
	jsonkv "github.com/ozontech/framer/formats/internal/kv/json"
	"github.com/ozontech/framer/formats/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Encoder struct {
	desriptors    reflection.DynamicMessagesStore
	metaMarshaler kv.Marshaler
}

func NewEncoder(desriptors reflection.DynamicMessagesStore) *Encoder {
	return &Encoder{desriptors, jsonkv.NewMultiVal()}
}

func (encoder *Encoder) MarshalAppend(b []byte, d *model.Data) ([]byte, error) {
	message, release := encoder.desriptors.Get(d.Method)
	if message == nil {
		return nil, fmt.Errorf("no such method: %s", d.Method)
	}
	defer release()

	b = append(b, `{"tag":`...)
	b = json.EscapeStringAppend(b, d.Tag)
	b = append(b, `,"call":`...)
	b = json.EscapeStringAppend(b, methodToJSON(d.Method))
	b = append(b, `,"metadata":`...)
	b = encoder.metaMarshaler.MarshalAppend(b, d.Metadata)
	b = append(b, `,"payload":`...)

	if err := proto.Unmarshal(d.Message, message); err != nil {
		return b, fmt.Errorf("unmarshaling binary payload for %s: %w", d.Method, err)
	}
	b, err := protojson.MarshalOptions{}.MarshalAppend(b, message)
	if err != nil {
		return b, fmt.Errorf("protojson marshal: %w", err)
	}
	return append(b, '}'), nil
}

// Приводит формат имени метода к виду используемому в либе dynamic - package.Service.Call
// в запросах используется каноничный fqn - /package.Service/Call.
func methodToJSON(method []byte) []byte {
	if len(method) == 0 {
		return method
	}

	if method[0] == '/' {
		method = method[1:]
	}

	ind := bytes.LastIndexByte(method, '/')
	if ind != -1 {
		method[ind] = '.'
	}

	return method
}

var _ model.Marshaler = &Encoder{}
