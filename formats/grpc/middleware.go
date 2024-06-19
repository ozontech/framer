package grpc

import (
	"github.com/ozontech/framer/formats/model"
)

// MiddlewareFunc позволяет выполнять модификации над запросами во время кодирования/декодирования.
type MiddlewareFunc func(*model.Data) *model.Data

// WrapEncoder оборачивает Marshaler, выполняя заданные модификации над запросами перед каждым вызовом MarshalAppend.
func WrapEncoder(enc model.Marshaler, mw ...MiddlewareFunc) model.Marshaler {
	if len(mw) == 0 {
		return enc
	}
	return &middlewareEncoder{
		enc: enc,
		mws: mw,
	}
}

type middlewareEncoder struct {
	enc model.Marshaler
	mws []MiddlewareFunc
}

func (e *middlewareEncoder) MarshalAppend(b []byte, data *model.Data) ([]byte, error) {
	for _, mw := range e.mws {
		data = mw(data)
	}
	return e.enc.MarshalAppend(b, data)
}
