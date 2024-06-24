package pool_test

import (
	"testing"

	"github.com/ozontech/framer/loader/streams/pool"
	"github.com/ozontech/framer/report/noop"
)

func BenchmarkStreamsPool(b *testing.B) {
	p := pool.NewStreamsPool(noop.New())
	for i := 0; i < b.N; i++ {
		s := p.Acquire(0, "")
		s.End()
	}
}
