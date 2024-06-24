package datasource

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/formats/model"
	hpackwrapper "github.com/ozontech/framer/utils/hpack_wrapper"
)

func TestFrameHeadersPool(t *testing.T) {
	t.Parallel()
	a := assert.New(t)
	frameHeaders := newFrameHeaders()
	buf1 := frameHeaders.Get()
	a.Len(buf1, 9)
	for i := range buf1 {
		buf1[i] = 1
	}

	buf2 := frameHeaders.Get()
	a.Len(buf2, 9)
	for i := range buf2 {
		buf2[i] = 2
	}

	for i := range buf1 {
		a.Equal(byte(1), buf1[i])
	}

	frameHeaders.Reset()

	buf3 := frameHeaders.Get()
	a.Len(buf3, 9)
	for i := range buf3 {
		buf3[i] = 3
	}

	// It reuses buf after reset
	for i := range buf1 {
		a.Equal(byte(3), buf1[i])
	}
}

func TestRequest1(t *testing.T) {
	t.Parallel()
	a := assert.New(t)
	r := NewRequestAdapter(
		func(string) bool { return true },
		[]string{":method", "POST", ":authority", ":authority-v"},
		[]string{"regular-k", "regular-v"},
	)
	buf := bytes.NewBuffer(nil)
	framer := http2.NewFramer(nil, buf)
	framer.ReadMetaHeaders = hpack.NewDecoder(4098, nil)

	const interations = 10
	for i := 0; i < interations; i++ {
		message := []byte("this is message")
		r.data = model.Data{
			Tag:    []byte("tag"),
			Method: []byte("/method"),
			Metadata: []model.Meta{
				{Name: []byte("k1"), Value: []byte("v1")},
				{Name: []byte("k2"), Value: []byte("v2")},
			},
			Message: message,
		}
		a.Equal(r.FullMethodName(), "/method")
		a.Equal(r.Tag(), "tag")

		hpw := hpackwrapper.NewWrapper()
		const streamID uint32 = 123
		frames := r.SetUp(consts.DefaultMaxFrameSize, streamID, hpw)
		a.Len(frames, 2)
		for _, f := range frames {
			for _, c := range f.Chunks {
				if c != nil {
					buf.Write(c)
				}
			}
		}

		// headers
		{
			f := frames[0]
			a.Zero(f.FlowControlPrice)

			expectedHeaders := []hpack.HeaderField{
				{Name: ":path", Value: "/method"},
				{Name: ":method", Value: "POST"},
				{Name: ":authority", Value: ":authority-v"},
				{Name: "regular-k", Value: "regular-v"},
				{Name: "k1", Value: "v1"},
				{Name: "k2", Value: "v2"},
			}
			http2Frame, err := framer.ReadFrame()
			a.NoError(err)
			mhf := http2Frame.(*http2.MetaHeadersFrame)

			a.Equal(expectedHeaders, mhf.Fields)
			header := mhf.Header()
			a.Equal(http2.FrameHeaders, header.Type)
			a.Equal(http2.FlagHeadersEndHeaders, header.Flags)
			a.Equal(streamID, header.StreamID)
		}

		// data
		{
			data := []byte{0}
			data = binary.BigEndian.AppendUint32(data, uint32(len(message)))
			data = append(data, message...)

			f := frames[0]
			a.Zero(f.FlowControlPrice)

			http2Frame, err := framer.ReadFrame()
			a.NoError(err)
			df := http2Frame.(*http2.DataFrame)

			header := df.Header()
			a.Equal(http2.FrameData, header.Type)
			a.Equal(http2.FlagDataEndStream, header.Flags)
			a.Equal(uint32(len(data)), header.Length)
			a.Equal(streamID, header.StreamID)
			a.Equal(data, df.Data())
		}
	}
}
