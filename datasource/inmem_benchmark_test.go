package datasource

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/loader/types"
)

func BenchmarkInmemDataSource(b *testing.B) {
	f, err := os.Open("../test_files/requests")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	ds := NewInmemDataSource(f)
	assert.NoError(b, ds.Init())

	rr := make(chan types.Req, 100)
	b.ResetTimer()
	go func() {
		defer close(rr)
		for i := 0; i < b.N; i++ {
			r, err := ds.Fetch()
			if err != nil {
				b.Error(err)
				return
			}
			rr <- r
		}
	}()

	for r := range rr {
		r.SetUp(consts.DefaultMaxFrameSize, 0, &noopHpackFieldWriter{})
		b.SetBytes(int64(r.Size()))
		r.Release()
	}
}
