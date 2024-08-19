package hpackwrapper

import (
	"io"

	"golang.org/x/net/http2/hpack"
)

type Wrapper struct {
	io.Writer
	enc *hpack.Encoder
}

func NewWrapper(opts ...Opt) *Wrapper {
	wrapper := &Wrapper{}
	wrapper.enc = hpack.NewEncoder(wrapper)
	for _, o := range opts {
		o.apply(wrapper)
	}

	return wrapper
}

func (ww *Wrapper) SetWriter(w io.Writer) { ww.Writer = w }
func (ww *Wrapper) WriteField(k, v string) {
	//nolint:errcheck // всегда пишем в буфер, это безопасно
	ww.enc.WriteField(hpack.HeaderField{
		Name:  k,
		Value: v,
	})
}

type Opt interface {
	apply(*Wrapper)
}

type WithMaxDynamicTableSize uint32

func (s WithMaxDynamicTableSize) apply(w *Wrapper) {
	w.enc.SetMaxDynamicTableSize(uint32(s))
}
