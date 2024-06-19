package json

import (
	"io"

	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	formatIO "github.com/ozontech/framer/formats/grpc/ozon/json/io"
	"github.com/ozontech/framer/formats/internal/pooledreader"
	"github.com/ozontech/framer/formats/model"
)

func NewInput(
	r io.Reader,
	store reflection.DynamicMessagesStore,
) *model.InputFormat {
	return &model.InputFormat{
		Reader:  pooledreader.New(formatIO.NewReader(r)),
		Decoder: encoding.NewDecoder(store),
	}
}

func NewOutput(
	w io.Writer,
	store reflection.DynamicMessagesStore,
) *model.OutputFormat {
	return &model.OutputFormat{
		Writer:  formatIO.NewWriter(w),
		Encoder: encoding.NewEncoder(store),
	}
}
