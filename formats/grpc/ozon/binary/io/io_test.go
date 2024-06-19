package io

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type test struct {
	name  string
	bytes []byte
	data  [][]byte
}

func makeTests() []test {
	return []test{
		{
			"simple",
			[]byte(`305
/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456
305
/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456
`),
			[][]byte{
				[]byte(`/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456`),
				[]byte(`/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456`),
			},
		},
	}
}

func TestReader(t *testing.T) {
	t.Parallel()

	runTest := func(tc test) {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a := assert.New(t)

			r := NewReader(bytes.NewReader(tc.bytes))
			var i int
			for {
				d, err := r.ReadNext(nil)
				if errors.Is(err, io.EOF) {
					break
				}
				a.NoError(err)

				var expected string
				if i < len(tc.data) {
					expected = string(tc.data[i])
				}
				assert.Equal(t, expected, string(d), fmt.Sprintf("read %d request", i))
				i++
			}
			assert.Equal(t, len(tc.data), i)
		})
	}

	for _, tc := range makeTests() {
		runTest(tc)
	}

	runTest(test{
		"without last \\n",
		[]byte(`305
/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456
305
/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456`),
		[][]byte{
			[]byte(`/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456`),
			[]byte(`/test.api.TestApi/Test
/test.api.TestApi/Test
{"user-agent":["grpcurl/v1.8.6 grpc-go/1.44.1-dev"],"x-forwarded-for":["1.2.3.4"],"x-forwarded-host":["myservice:9090"],"x-forwarded-port":["9090"],"x-forwarded-proto":["http"],"x-forwarded-scheme":["http"],"x-real-ip":["1.2.3.4"],"x-scheme":["http"]}

123456`),
		},
	})
}

func TestWriter(t *testing.T) {
	t.Parallel()
	for _, tc := range makeTests() {
		tc := tc

		bb := new(bytes.Buffer)
		t.Run(tc.name, func(t *testing.T) {
			a := assert.New(t)
			t.Parallel()
			bb.Reset()
			w := NewWriter(bb)
			for _, d := range tc.data {
				a.NoError(w.WriteNext(d))
			}
			assert.Equal(t, string(tc.bytes), bb.String())
		})
	}
}
