package binary

import (
	"io"

	"github.com/ozontech/framer/formats/grpc/ozon/binary/encoding"
	binaryIO "github.com/ozontech/framer/formats/grpc/ozon/binary/io"
	"github.com/ozontech/framer/formats/internal/pooledreader"
	"github.com/ozontech/framer/formats/model"
)

func NewInput(r io.Reader) *model.InputFormat {
	return &model.InputFormat{
		Reader:  pooledreader.New(binaryIO.NewReader(r)),
		Decoder: encoding.NewDecoder(),
	}
}

func NewOutput(w io.Writer) *model.OutputFormat {
	return &model.OutputFormat{
		Writer:  binaryIO.NewWriter(w),
		Encoder: encoding.NewEncoder(),
	}
}
