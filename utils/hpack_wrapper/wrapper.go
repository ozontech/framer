package hpackwrapper

import (
	"io"

	"golang.org/x/net/http2/hpack"
)

type Wrapper struct {
	io.Writer
	enc *hpack.Encoder
}

func NewWrapper() *Wrapper {
	wrapper := &Wrapper{}
	wrapper.enc = hpack.NewEncoder(wrapper)
	return wrapper
}

func (ww *Wrapper) SetWriter(w io.Writer) { ww.Writer = w }
func (ww *Wrapper) WriteField(k, v string) error {
	return ww.enc.WriteField(hpack.HeaderField{
		Name:  k,
		Value: v,
	})
}
