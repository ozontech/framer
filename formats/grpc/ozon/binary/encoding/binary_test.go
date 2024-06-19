package encoding_test

import (
	"testing"

	"github.com/ozontech/framer/formats/grpc/ozon/binary/encoding"
	"github.com/ozontech/framer/formats/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type test struct {
	name  string
	bytes []byte
	data  *model.Data
}

func makeTests() []test {
	return []test{{
		"simple",
		[]byte(
			"/Test\n" +
				"/test.api.TestApi/Test\n" +
				`{"key":["val1","val2"]}` + "\n" +
				"This is body\r\nmultiline\r\nbody\r\n\r\n",
		),
		&model.Data{
			Tag:    []byte("/Test"),
			Method: []byte("/test.api.TestApi/Test"),
			Metadata: []model.Meta{
				{Name: []byte("key"), Value: []byte("val1")},
				{Name: []byte("key"), Value: []byte("val2")},
			},
			Message: []byte("This is body\r\nmultiline\r\nbody\r\n\r\n"),
		},
	}}
}

func TestDecoder(t *testing.T) {
	t.Parallel()
	d := encoding.NewDecoder()
	for _, tc := range makeTests() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data := new(model.Data)
			require.NoError(t, d.Unmarshal(data, tc.bytes))
			assert.Equal(t, tc.data, data)
		})
	}
}

func TestEncoder(t *testing.T) {
	t.Parallel()
	e := encoding.NewEncoder()
	for _, tc := range makeTests() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := assert.New(t)
			b, err := e.MarshalAppend(nil, tc.data)
			a.NoError(err)
			a.Equal(string(tc.bytes), string(b))
		})
	}
}
