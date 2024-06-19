package datasource

import (
	"bytes"
	"strings"
	"time"
	"unsafe"

	"github.com/ozontech/framer/formats/model"
	"github.com/ozontech/framer/frameheader"
	"github.com/ozontech/framer/loader/types"
	grpcutil "github.com/ozontech/framer/utils/grpc"
	"golang.org/x/net/http2"
)

type Option interface {
	apply(f *RequestAdapterFactory)
}

type RequestAdapterFactory struct {
	staticPseudoHeaders  []string
	staticRegularHeaders []string
	headerFilter         func(k string) (mustSkip bool)
}

func NewRequestAdapterFactory(ops ...Option) *RequestAdapterFactory {
	f := &RequestAdapterFactory{
		headerFilter: func(k string) (allowed bool) { return true },
		staticPseudoHeaders: []string{
			":method", "POST",
			":scheme", "http",
		},
		staticRegularHeaders: []string{
			"content-type", "application/grpc",
			"te", "trailers",
		},
	}
	for _, o := range ops {
		o.apply(f)
	}
	return f
}

type additionalHeadersOpts []string

func (h additionalHeadersOpts) apply(f *RequestAdapterFactory) {
	for i := 0; i < len(h); i += 2 {
		k, v := h[i], h[i+1]
		if strings.HasPrefix(k, ":") {
			f.staticPseudoHeaders = append(f.staticPseudoHeaders, k, v)
		} else {
			f.staticRegularHeaders = append(f.staticRegularHeaders, k, v)
		}
	}
}

func WithAdditionalHeader(k, v string) Option {
	return additionalHeadersOpts([]string{k, v})
}

func WithAdditionalHeaders(headers []string) Option {
	return additionalHeadersOpts(headers)
}

func WithTimeout(t time.Duration) Option {
	return additionalHeadersOpts([]string{"grpc-timeout", grpcutil.EncodeDuration(t)})
}

func (f *RequestAdapterFactory) Build() *RequestAdapter {
	return NewRequestAdapter(
		f.isAllowedMeta,
		f.staticPseudoHeaders,
		f.staticRegularHeaders,
	)
}

func (f *RequestAdapterFactory) isAllowedMeta(k string) (allowed bool) {
	if k == "" {
		return false
	}
	// отфильтровываем псевдохедеры из меты т.к.
	// стандартный клиент также псевдохедеры не пропускает
	// и псевдохедерами можно легко сломать стрельбу
	if k[0] == ':' {
		return false
	}
	switch k {
	case "content-type", "te", "grpc-timeout":
		return false
	}
	return f.headerFilter(k)
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
	isAllowedMeta func(k string) bool
	staticPseudo  []string
	staticRegular []string
	size          int

	frameHeaders  *frameHeaders
	payloadPrefix [5]byte
	frames        []types.Frame
	headersBuf    *bytes.Buffer
	data          model.Data
}

func NewRequestAdapter(
	isAllowedMeta func(k string) bool,
	staticPseudo []string,
	staticRegular []string,
) *RequestAdapter {
	return &RequestAdapter{
		frameHeaders:  newFrameHeaders(),
		frames:        make([]types.Frame, 2),
		isAllowedMeta: isAllowedMeta,
		staticPseudo:  staticPseudo,
		staticRegular: staticRegular,
		headersBuf:    bytes.NewBuffer(nil),
	}
}

func (a *RequestAdapter) setData(data model.Data) { a.data = data }
func (a *RequestAdapter) FullMethodName() string  { return unsafeString(a.data.Method) }
func (a *RequestAdapter) Tag() string             { return unsafeString(a.data.Tag) }

const maxFramePayloadLen int = 16384 // Максимальная длина пейлоада фрейма в grpc. У http2 ограничение больше.

// TODO(pgribanov): после реализации собственной системы энкодинга хедеров,
// отказаться от unsafe
func unsafeString(b []byte) string {
	//nolint:gosec
	return unsafe.String(&b[0], len(b))
}

func (a *RequestAdapter) setUpHeaders(
	streamID uint32,
	hpack types.HPackFieldWriter,
) {
	a.size = 0
	hpack.SetWriter(a.headersBuf)

	data := a.data
	//nolint:errcheck // пишем в буфер, это безопасно
	hpack.WriteField(":path", unsafeString(data.Method))

	// добавляем статичные псевдохедеры
	staticPseudo := a.staticPseudo
	for i := 0; i < len(staticPseudo); i += 2 {
		//nolint:errcheck // пишем в буфер, это безопасно
		hpack.WriteField(staticPseudo[i], staticPseudo[i+1])
	}

	// добавляем статичные хедеры
	staticRegular := a.staticRegular
	for i := 0; i < len(staticRegular); i += 2 {
		//nolint:errcheck // пишем в буфер, это безопасно
		hpack.WriteField(staticRegular[i], staticRegular[i+1])
	}

	// добавляем мету из запроса
	for _, m := range data.Metadata {
		k := unsafeString(m.Name)
		v := unsafeString(m.Value)
		if !a.isAllowedMeta(k) {
			continue
		}

		//nolint:errcheck // пишем в буфер, это безопасно
		hpack.WriteField(k, v)
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
}

func (a *RequestAdapter) setUpPayload(
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
	streamID uint32,
	hpackFieldWriter types.HPackFieldWriter,
) []types.Frame {
	a.headersBuf.Reset()
	a.frameHeaders.Reset()

	a.frames = a.frames[:0]

	a.setUpHeaders(streamID, hpackFieldWriter)
	a.setUpPayload(streamID)

	return a.frames
}
