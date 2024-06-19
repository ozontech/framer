package grpc

import (
	"github.com/ozontech/framer/formats/model"
	"github.com/ozontech/framer/utils/pool"
)

type DataHolder struct {
	data     model.Data
	readBuf  []byte
	writeBuf []byte
}

type ConvertStrategy struct {
	pool *pool.SlicePool[*DataHolder]

	in  *model.InputFormat
	out *model.OutputFormat
}

func NewConvertStrategy(in *model.InputFormat, out *model.OutputFormat) *ConvertStrategy {
	return &ConvertStrategy{
		pool: pool.NewSlicePool[*DataHolder](),
		in:   in,
		out:  out,
	}
}

func (s *ConvertStrategy) Read() (interface{}, error) {
	dh, ok := s.pool.Acquire()
	if !ok {
		dh = new(DataHolder)
	}

	var err error
	dh.readBuf, err = s.in.Reader.ReadNext()
	return dh, err
}

func (s *ConvertStrategy) Decode(dataHolder interface{}) error {
	dh := dataHolder.(*DataHolder)
	return s.in.Decoder.Unmarshal(&dh.data, dh.readBuf)
}

func (s *ConvertStrategy) Encode(dataHolder interface{}) error {
	dh := dataHolder.(*DataHolder)
	var err error
	dh.writeBuf, err = s.out.Encoder.MarshalAppend(dh.writeBuf, &dh.data)
	return err
}

func (s *ConvertStrategy) Write(dataHolder interface{}) error {
	dh := dataHolder.(*DataHolder)
	defer s.in.Reader.Release(dh.readBuf)
	return s.out.Writer.WriteNext(dh.writeBuf)
}
