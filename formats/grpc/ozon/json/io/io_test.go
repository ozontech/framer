package io

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type test struct {
	name  string
	bytes []byte
	data  [][]byte
}

//nolint:goconst
func makeTests() []test {
	return []test{{
		"simple",
		[]byte(
			"line1\n" +
				"line2\n" +
				"line3\n" +
				"line4\n" +
				"line5\n" +
				"line6\n",
		),
		[][]byte{
			[]byte("line1"),
			[]byte("line2"),
			[]byte("line3"),
			[]byte("line4"),
			[]byte("line5"),
			[]byte("line6"),
		},
	}}
}

func TestReader(t *testing.T) {
	t.Parallel()
	tests := append(makeTests(), test{
		"without last \\n",
		[]byte(
			"line1\n" +
				"line2\n" +
				"line3\n" +
				"line4\n" +
				"line5\n" +
				"line6",
		),
		[][]byte{
			[]byte("line1"),
			[]byte("line2"),
			[]byte("line3"),
			[]byte("line4"),
			[]byte("line5"),
			[]byte("line6"),
		},
	})
	for _, tc := range tests {
		tc := tc

		r := NewReader(bufio.NewReader(bytes.NewReader(tc.bytes)))
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := assert.New(t)
			var i int
			for {
				d, err := r.ReadNext(nil)
				if errors.Is(err, io.EOF) {
					break
				}
				a.NoError(err)
				var data string
				if i < len(tc.data) {
					data = string(tc.data[i])
				}
				a.Equal(data, string(d))
				i++
			}
			a.Equal(len(tc.data), i)
		})
	}
}

func TestWriter(t *testing.T) {
	t.Parallel()
	for _, tc := range makeTests() {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := assert.New(t)

			bb := new(bytes.Buffer)
			w := NewWriter(bb)
			for _, d := range tc.data {
				a.NoError(w.WriteNext(d))
			}

			assert.Equal(t, string(tc.bytes), bb.String())
		})
	}
}
