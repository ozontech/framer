package datasource

import (
	"bytes"
	"fmt"
	"time"

	"github.com/ozontech/framer/datasource/decoder"
	"github.com/ozontech/framer/frameheader"
	"github.com/ozontech/framer/loader/types"
	grpcutil "github.com/ozontech/framer/utils/grpc"
	"golang.org/x/net/http2"
)

type config struct {
	additionalHeaders []string
	metaMiddleware    MetaMiddleware
}

type Option interface {
	apply(f *config)
}

type RequestAdapterFactory struct {
	metaMiddleware MetaMiddleware
}

func NewRequestAdapterFactory(ops ...Option) *RequestAdapterFactory {
	c := config{
		metaMiddleware: noopMiddleware{},
	}
	for _, o := range ops {
		o.apply(&c)
	}

	return &RequestAdapterFactory{
		metaMiddleware: newDefaultMiddleware(c.metaMiddleware, c.additionalHeaders...),
	}
}

func (f *RequestAdapterFactory) Build() *RequestAdapter {
	return NewRequestAdapter(f.metaMiddleware)
}

type frameHeaders []byte

func newFrameHeaders() *frameHeaders {
	fhs := make(frameHeaders, 0, 3*9)
	return &fhs
}
func (fp *frameHeaders) Reset() { *fp = (*fp)[:0] }
func (fp *frameHeaders) Get() frameheader.FrameHeader {
	f := *fp
	l := len(f)
	if cap(f) < l+9 {
		f = append(f, make([]byte, 9)...)
	} else {
		f = f[:l+9]
	}
	*fp = f
	return frameheader.FrameHeader(f[l : l+9])
}

type RequestAdapter struct {
	metaMiddleware MetaMiddleware
	size           int

	frameHeaders  *frameHeaders
	payloadPrefix [5]byte
	frames        []types.Frame
	headersBuf    *bytes.Buffer
	data          decoder.Data
}

func NewRequestAdapter(metaMiddleware MetaMiddleware) *RequestAdapter {
	return &RequestAdapter{
		frameHeaders:   newFrameHeaders(),
		frames:         make([]types.Frame, 2),
		metaMiddleware: metaMiddleware,
		headersBuf:     bytes.NewBuffer(nil),
	}
}

func (a *RequestAdapter) setData(data decoder.Data) { a.data = data }
func (a *RequestAdapter) FullMethodName() string    { return a.data.Method }
func (a *RequestAdapter) Tag() string               { return a.data.Tag }

func (a *RequestAdapter) setUpHeaders(
	maxFramePayloadLen int,
	maxHeaderListSize int, // https://datatracker.ietf.org/doc/html/rfc9113#section-6.5.2-2.12.1
	streamID uint32,
	hpack types.HPackFieldWriter,
) error {
	a.size = 0
	var headerListSize int // https://datatracker.ietf.org/doc/html/rfc9113#section-6.5.2-2.12.1
	hpack.SetWriter(a.headersBuf)

	data := a.data
	headerListSize += len(":path") + len(data.Method)
	hpack.WriteField(":path", data.Method)
	a.metaMiddleware.WriteAdditional(hpack)

	// добавляем мету из запроса
	for _, m := range data.Metadata {
		if !a.metaMiddleware.IsAllowed(m.Name) {
			continue
		}
		headerListSize += len(m.Name) + len(m.Value)
		hpack.WriteField(m.Name, m.Value)
	}
	if headerListSize > maxHeaderListSize {
		return fmt.Errorf("header list size exeed limit: %d > %d", headerListSize, maxHeaderListSize)
	}

	for {
		b := a.headersBuf.Next(maxFramePayloadLen)
		bLen := len(b)
		endHeaders := bLen < maxFramePayloadLen

		frameHeader := a.frameHeaders.Get()
		frameHeader.SetLength(bLen)
		if len(a.frames) == 0 {
			frameHeader.SetType(http2.FrameHeaders)
		} else {
			frameHeader.SetType(http2.FrameContinuation)
		}
		if endHeaders {
			frameHeader.SetFlags(http2.FlagHeadersEndHeaders)
		} else {
			frameHeader.SetFlags(0)
		}
		frameHeader.SetStreamID(streamID)

		a.frames = append(a.frames, types.Frame{
			Chunks: [3][]byte{frameHeader, b},
		})

		a.size += 9 + bLen
		if bLen < maxFramePayloadLen {
			break
		}
	}
	return nil
}

func (a *RequestAdapter) setUpPayload(
	maxFramePayloadLen int,
	streamID uint32,
) {
	data := a.data
	pl := data.Message
	plLen := len(pl)

	_ = a.payloadPrefix[4]
	a.payloadPrefix[1] = byte(plLen >> 24)
	a.payloadPrefix[2] = byte(plLen >> 16)
	a.payloadPrefix[3] = byte(plLen >> 8)
	a.payloadPrefix[4] = byte(plLen)

	bLen := min(len(pl), maxFramePayloadLen-5)
	b := pl[:bLen]
	pl = pl[bLen:]
	framePayloadLen := bLen + 5

	var flags http2.Flags
	frameHeader := a.frameHeaders.Get()
	if len(pl) == 0 {
		flags = http2.FlagDataEndStream
	}
	frameHeader.Fill(framePayloadLen, http2.FrameData, flags, streamID)

	frame := types.Frame{
		Chunks: [3][]byte{
			frameHeader, a.payloadPrefix[:], b,
		},
		FlowControlPrice: uint32(framePayloadLen),
	}

	a.size += 9 + 5 + bLen
	if len(pl) == 0 {
		a.frames = append(a.frames, frame)
		return
	}

	a.frames = append(a.frames, frame)

	for {
		bLen := min(len(pl), maxFramePayloadLen)
		b := pl[:bLen]
		pl = pl[bLen:]
		endData := bLen < maxFramePayloadLen

		frameHeader := a.frameHeaders.Get()
		if endData {
			flags = http2.FlagDataEndStream
		} else {
			flags = 0
		}
		frameHeader.Fill(bLen, http2.FrameData, flags, streamID)

		frame = types.Frame{
			Chunks: [3][]byte{
				frameHeader, a.payloadPrefix[:], b,
			},
			FlowControlPrice: uint32(bLen),
		}

		a.size += 9 + bLen
		if len(pl) == 0 {
			a.frames = append(a.frames, frame)
			return
		}
		a.frames = append(a.frames, frame)
	}
}

func (a *RequestAdapter) Size() int {
	return a.size
}

func (a *RequestAdapter) SetUp(
	maxFramePayloadLen int,
	maxHeaderListSize int,
	streamID uint32,
	hpackFieldWriter types.HPackFieldWriter,
) ([]types.Frame, error) {
	a.headersBuf.Reset()
	a.frameHeaders.Reset()

	a.frames = a.frames[:0]

	err := a.setUpHeaders(maxFramePayloadLen, maxHeaderListSize, streamID, hpackFieldWriter)
	a.setUpPayload(maxFramePayloadLen, streamID)

	return a.frames, err
}

func WithAdditionalHeader(k, v string) Option {
	return fnOption(func(c *config) {
		c.additionalHeaders = append(c.additionalHeaders, k, v)
	})
}

func WithAdditionalHeaders(headers []string) Option {
	return fnOption(func(c *config) {
		c.additionalHeaders = append(c.additionalHeaders, headers...)
	})
}

func WithTimeout(t time.Duration) Option {
	return fnOption(func(c *config) {
		c.additionalHeaders = append(c.additionalHeaders, "grpc-timeout", grpcutil.EncodeDuration(t))
	})
}

func WithMetaMiddleware(mw MetaMiddleware) Option {
	return fnOption(func(c *config) { c.metaMiddleware = mw })
}

type fnOption func(c *config)

func (fn fnOption) apply(c *config) { fn(c) }
