package encoding

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/mailru/easyjson/jlexer"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	"github.com/ozontech/framer/formats/internal/kv"
	jsonkv "github.com/ozontech/framer/formats/internal/kv/json"
	"github.com/ozontech/framer/formats/model"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

type DecoderOption func(*Decoder)

func WithSingleMetaValue() DecoderOption {
	return func(r *Decoder) {
		r.metaUnmarshaler = jsonkv.NewSingleVal()
	}
}

type Decoder struct {
	desriptors      reflection.DynamicMessagesStore
	metaUnmarshaler kv.Unmarshaler
}

func NewDecoder(
	desriptors reflection.DynamicMessagesStore,
	opts ...DecoderOption,
) *Decoder {
	d := &Decoder{desriptors, jsonkv.NewMultiVal()}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Unmarshal unmarshal bytes to *grpc.Data struct
func (decoder *Decoder) Unmarshal(d *model.Data, b []byte) error {
	d.Reset()
	var err error

	in := jlexer.Lexer{Data: b}

	var (
		jsonPayload    []byte
		dynamicMessage *dynamicpb.Message
	)
	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeFieldName(true)
		in.WantColon()
		switch key {
		case "tag":
			d.Tag = in.UnsafeBytes()
		case "call":
			d.Method = methodFromJSON(in.UnsafeBytes())
			var release func()
			dynamicMessage, release = decoder.desriptors.Get(d.Method)
			if dynamicMessage == nil {
				return errors.New("no such method: " + string(d.Method))
			}
			defer release()
		case "metadata":
			d.Metadata, err = decoder.metaUnmarshaler.UnmarshalAppend(
				d.Metadata, in.Raw(),
			)
			if err != nil {
				return fmt.Errorf("unmarshal meta: %w", err)
			}
		case "payload":
			jsonPayload = in.Raw()
		default:
			return fmt.Errorf("unknown field: %s", key)
		}
		in.WantComma()
	}
	in.Delim('}')
	in.Consumed()

	if dynamicMessage == nil {
		return fmt.Errorf(`"call" is required`)
	}

	if err = protojson.Unmarshal(jsonPayload, dynamicMessage); err != nil {
		return fmt.Errorf(
			"unmarshalling json payload for %s: %w",
			d.Method, err,
		)
	}
	d.Message, err = proto.MarshalOptions{}.MarshalAppend(d.Message, dynamicMessage)
	if err != nil {
		return fmt.Errorf("marshaling grpc message into binary: %w", err)
	}

	return nil
}

// methodFromJSON - приводит метод к стандартному виду '/package.Service/Call'
// старый формат - 'package.Service.Call'.
func methodFromJSON(method []byte) []byte {
	if len(method) == 0 || method[0] == '/' {
		return method
	}

	ind := bytes.LastIndexByte(method, '.')
	if ind != -1 {
		method[ind] = '/'
	}
	method = append(method, 0x0)
	copy(method[1:], method)
	method[0] = '/'

	return method
}

var _ model.Unmarshaler = &Decoder{}
