package encoding_test

import (
	"fmt"
	"testing"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	tesproto "github.com/ozontech/framer/formats/grpc/ozon/json/encoding/testproto"
	"github.com/ozontech/framer/formats/model"
)

func mustMakeStore() reflection.DynamicMessagesStore {
	fds, err := protoparse.Parser{
		LookupImport: desc.LoadFileDescriptor,
		ImportPaths:  []string{"./testproto"},
	}.ParseFiles("service.proto")
	if err != nil {
		panic(fmt.Errorf("can't parse proto files: %w", err))
	}

	var methods []*desc.MethodDescriptor
	for _, fd := range fds {
		services := fd.GetServices()
		for _, service := range services {
			methods = append(methods, service.GetMethods()...)
		}
	}
	return reflection.NewDynamicMessagesStore(methods)
}

type test struct {
	name  string
	bytes []byte
	data  *model.Data
}

func makeTests() []test {
	return []test{{
		"simple",
		[]byte(
			"{" +
				`"tag":"tag",` +
				`"call":"testpackage.TestService.TestMethod5",` +
				`"metadata":{"key":["val1","val2"]},` +
				`"payload":{"request":"12345"}` +
				"}",
		),
		&model.Data{
			Tag:    []byte("tag"),
			Method: []byte("/testpackage.TestService/TestMethod5"),
			Metadata: []model.Meta{
				{Name: []byte("key"), Value: []byte("val1")},
				{Name: []byte("key"), Value: []byte("val2")},
			},
			Message: mustMarshal(&tesproto.TestRequest5{
				Request: "12345",
			}),
		},
	}}
}

func mustMarshal(m proto.Message) []byte {
	b, err := proto.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

func TestDecoder(t *testing.T) {
	t.Parallel()
	d := encoding.NewDecoder(mustMakeStore())
	for _, tc := range makeTests() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := new(model.Data)
			in := append([]byte{}, tc.bytes...) // bytes.Clone
			err := d.Unmarshal(data, in)
			if err != nil {
				t.Fatal(err)
				return
			}
			if !assert.Equal(t, tc.data, data) {
				t.Logf("%s\n", tc.data.Method)
			}
		})
	}

	t.Run("garbage in root", func(t *testing.T) {
		t.Parallel()
		data := new(model.Data)
		in := []byte("garbage")
		assert.Error(t, d.Unmarshal(data, in))
	})

	t.Run("garbage in payload", func(t *testing.T) {
		t.Parallel()
		data := new(model.Data)
		in := []byte(
			"{" +
				`"tag":"tag",` +
				`"call":"testpackage.TestService.TestMethod5",` +
				`"metadata":{"key":["val1","val2"]},` +
				`"payload":"garbage"` +
				"}",
		)
		assert.Error(t, d.Unmarshal(data, in))
	})
}

func TestEncoder(t *testing.T) {
	t.Parallel()
	e := encoding.NewEncoder(mustMakeStore())
	for _, tc := range makeTests() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := e.MarshalAppend(nil, tc.data)
			if err != nil {
				t.Fatal(err)
				return
			}
			assert.Equal(t, string(tc.bytes), string(b))
		})
	}

	t.Run("garbage in payload", func(t *testing.T) {
		t.Parallel()
		_, err := e.MarshalAppend(nil, &model.Data{
			Tag:    []byte("tag"),
			Method: []byte("/testpackage.TestService/TestMethod5"),
			Metadata: []model.Meta{
				{Name: []byte("key"), Value: []byte("val1")},
				{Name: []byte("key"), Value: []byte("val2")},
			},
			Message: []byte("garbagebytes"),
		})
		assert.Error(t, err)
	})
}
