package datasource

import (
	"fmt"
	"io"
	"sync"

	"github.com/ozontech/framer/formats/grpc/ozon/binary"
	"github.com/ozontech/framer/formats/model"
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
)

type FileDataSource struct {
	format  *model.InputFormat
	pool    *pool.SlicePool[*fileRequest]
	factory *RequestAdapterFactory
	mu      sync.Mutex
}

func NewFileDataSource(r io.Reader, factoryOptions ...Option) *FileDataSource {
	ds := &FileDataSource{
		binary.NewInput(r),
		pool.NewSlicePoolSize[*fileRequest](100),
		NewRequestAdapterFactory(factoryOptions...),
		sync.Mutex{},
	}
	return ds
}

func (ds *FileDataSource) Fetch() (types.Req, error) {
	r, ok := ds.pool.Acquire()
	if !ok {
		r = &fileRequest{
			bytesPool:      ds.format.Reader,
			pool:           ds.pool,
			RequestAdapter: ds.factory.Build(),
		}
	}

	var err error
	ds.mu.Lock()
	r.bytes, err = ds.format.Reader.ReadNext()
	ds.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("read next request: %w", err)
	}

	return r, ds.format.Decoder.Unmarshal(&r.data, r.bytes)
}

type fileRequest struct {
	bytesPool model.PooledRequestReder
	bytes     []byte
	pool      *pool.SlicePool[*fileRequest]
	*RequestAdapter
}

func (r *fileRequest) Release() {
	r.bytesPool.Release(r.bytes)
	r.pool.Release(r)
}
