//nolint:dupl
package jsonkv

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ozontech/framer/formats/model"
)

var multiValTests = []struct {
	name  string
	bytes []byte
	data  []model.Meta
}{
	{
		"simple",
		[]byte(`{"key":["val"]}`),
		[]model.Meta{
			{Name: []byte("key"), Value: []byte("val")},
		},
	},
	{
		"json encoded strings",
		[]byte(`{"key\n":["val\n"]}`),
		[]model.Meta{
			{Name: []byte("key\n"), Value: []byte("val\n")},
		},
	},
	{
		"duplicate vals",
		[]byte(`{"key":["val","val"]}`),
		[]model.Meta{
			{Name: []byte("key"), Value: []byte("val")},
			{Name: []byte("key"), Value: []byte("val")},
		},
	},
}

func TestMultiValDecoder(t *testing.T) {
	t.Parallel()
	d := NewMultiVal()

	for _, tc := range multiValTests {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := d.UnmarshalAppend(nil, tc.bytes)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, tc.data, out)
		})

		t.Run("append/"+tc.name, func(t *testing.T) {
			t.Parallel()
			m := model.Meta{Name: []byte("exist key"), Value: []byte("exist val")}
			buf := []model.Meta{m}
			out, err := d.UnmarshalAppend(buf, tc.bytes)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, append(
				[]model.Meta{{Name: []byte("exist key"), Value: []byte("exist val")}},
				tc.data...,
			), out)
		})
	}

	t.Run("error", func(t *testing.T) {
		t.Parallel()
		_, err := d.UnmarshalAppend(nil, []byte(`some garbage{///]`))
		assert.Error(t, err)
	})
}
