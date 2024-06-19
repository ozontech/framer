package json

import (
	"io"

	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding"
	"github.com/ozontech/framer/formats/grpc/ozon/json/encoding/reflection"
	retuestIO "github.com/ozontech/framer/formats/grpc/ozon/json/io"
	"github.com/ozontech/framer/formats/internal/pooledreader"
	"github.com/ozontech/framer/formats/model"
)

func NewInput(r io.Reader, store reflection.DynamicMessagesStore) *model.InputFormat {
	return &model.InputFormat{
		Reader:  pooledreader.New(retuestIO.NewReader(r)),
		Decoder: encoding.NewDecoder(store, encoding.WithSingleMetaValue()),
	}
}
