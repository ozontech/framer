package datasource

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/ozontech/framer/datasource/decoder"
	"github.com/ozontech/framer/formats/grpc/ozon/binary"
	"github.com/ozontech/framer/formats/model"
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
)

type InmemDataSource struct {
	reader  model.PooledRequestReader
	decoder *decoder.Decoder

	factory *RequestAdapterFactory
	pool    *pool.SlicePool[*memRequest]
	i       atomic.Int32
	datas   []decoder.Data
}

func NewInmemDataSource(r io.Reader, factoryOptions ...Option) *InmemDataSource {
	return &InmemDataSource{
		binary.NewInput(r).Reader,
		decoder.NewDecoder(),

		NewRequestAdapterFactory(factoryOptions...),
		pool.NewSlicePoolSize[*memRequest](100),
		atomic.Int32{},
		nil,
	}
}

func (ds *InmemDataSource) Init() error {
	var b []byte
	var err error
	for {
		b, err = ds.reader.ReadNext()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("read next request: %w", err)
		}
		var data decoder.Data
		err = ds.decoder.Unmarshal(&data, b)
		if err != nil {
			return fmt.Errorf("read next request: %w", err)
		}
		ds.datas = append(ds.datas, data)
	}
	if len(ds.datas) == 0 {
		return errors.New("request file is empty")
	}
	return nil
}

func (ds *InmemDataSource) Fetch() (types.Req, error) {
	r, ok := ds.pool.Acquire()
	if !ok {
		adapter := ds.factory.Build()
		r = &memRequest{
			pool:           ds.pool,
			RequestAdapter: adapter,
		}
	}

	i := int(ds.i.Add(1))
	r.setData(ds.datas[i%len(ds.datas)])
	return r, nil
}

type memRequest struct {
	pool *pool.SlicePool[*memRequest]
	*RequestAdapter
}

func (r *memRequest) Release() { r.pool.Release(r) }
