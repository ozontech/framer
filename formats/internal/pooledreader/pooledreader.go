package pooledreader

import (
	"github.com/ozontech/framer/formats/model"
	"github.com/ozontech/framer/utils/pool"
)

type PooledReader struct {
	pool *pool.SlicePool[[]byte]
	r    model.RequestReader
}

func New(r model.RequestReader) *PooledReader {
	return &PooledReader{pool.NewSlicePool[[]byte](), r}
}

func (r *PooledReader) ReadNext() ([]byte, error) {
	b, _ := r.pool.Acquire()
	return r.r.ReadNext(b[:0])
}

func (r *PooledReader) Release(b []byte) {
	r.pool.Release(b)
}
