package reciever

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ozontech/framer/frameheader"
	"github.com/ozontech/framer/loader/types"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

var ErrFrameTypeNotSupported = errors.New("frame type not supported")

type FrameTypeProcessor interface {
	Process(header frameheader.FrameHeader, payload []byte, incomplete bool) error
}

type Processor struct {
	framer        *Framer
	subprocessors []FrameTypeProcessor
}

func NewProcessor(subprocessors []FrameTypeProcessor) *Processor {
	return &Processor{new(Framer), subprocessors}
}

func NewDefaultProcessor(
	streamsStore types.StreamStore,
	streamsLimiter types.StreamsLimiter,
	fcConn types.FlowControl,
	priorityFramesChan chan<- []byte,
) *Processor {
	headersFrameProcessor := newHeadersFrameProcessor(streamsStore, streamsLimiter)
	return NewProcessor([]FrameTypeProcessor{
		http2.FrameData:    newDataFrameProcessor(priorityFramesChan, streamsStore, streamsLimiter),
		http2.FrameHeaders: headersFrameProcessor,
		// http2.FramePriority not supported
		http2.FrameRSTStream: newRSTStreamFrameProcessor(streamsStore, streamsLimiter),
		http2.FrameSettings:  settingsProcessor{},
		// http2.FramePushPromise not supported
		http2.FramePing:         newPingFrameProcessor(priorityFramesChan),
		http2.FrameGoAway:       newGoAwayFrameProcessor(),
		http2.FrameWindowUpdate: newWindowUpdateFrameProcessor(streamsStore, fcConn),
		http2.FrameContinuation: headersFrameProcessor,
	})
}

func (p *Processor) Run(ch <-chan []byte) error {
	for b := range ch {
		err := p.process(b)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) process(buf []byte) error {
	var (
		err    error
		b      []byte
		status Status
		header frameheader.FrameHeader
	)
	p.framer.Fill(buf)
	for {
		b, status = p.framer.Next()
		if status == StatusHeaderIncomplete {
			return nil
		}

		header = p.framer.Header()
		// if header.Length() > 100 {
		// 	panic(header.String())
		// }

		sp := p.subprocessors[header.Type()]
		if sp == nil {
			return fmt.Errorf("%w: %s", ErrFrameTypeNotSupported, header.Type().String())
		}

		err = sp.Process(header, b, status == StatusPayloadIncomplete)
		if err != nil {
			return err
		}

		if status == StatusFrameDone {
			continue
		}

		return nil
	}
}

type pingFrameProcessor struct {
	outFramesChan       chan<- []byte
	currentPingAckFrame []byte
	nextPingAckFrame    []byte
}

func newPingFrameProcessor(outFramesChan chan<- []byte) *pingFrameProcessor {
	pingAckFrame := [9 + 8]byte{0, 0, 8, byte(http2.FramePing), byte(http2.FlagPingAck)}
	return &pingFrameProcessor{
		outFramesChan,
		pingAckFrame[:9],
		bytes.Clone(pingAckFrame[:9]),
	}
}

func (p *pingFrameProcessor) Process(_ frameheader.FrameHeader, payload []byte, incomplete bool) error {
	p.currentPingAckFrame = append(p.currentPingAckFrame, payload...)
	if incomplete {
		return nil
	}
	p.outFramesChan <- p.currentPingAckFrame
	p.currentPingAckFrame, p.nextPingAckFrame = p.nextPingAckFrame[:9], p.currentPingAckFrame
	return nil
}

type dataFrameProcessor struct {
	outFramesChan  chan<- []byte
	streamsStore   types.StreamStore
	streamsLimiter types.StreamsLimiter

	currentWindowUpdateFrame []byte
	nextWindowUpdateFrame    []byte
	windowUpdateAcc          int
}

func newDataFrameProcessor(
	outFramesChan chan<- []byte,
	streamsStore types.StreamStore,
	streamsLimiter types.StreamsLimiter,
) *dataFrameProcessor {
	windowUpdateFrame := [9 + 4]byte{0, 0, 4, byte(http2.FrameWindowUpdate)}
	return &dataFrameProcessor{
		outFramesChan,
		streamsStore,
		streamsLimiter,
		windowUpdateFrame[:],
		bytes.Clone(windowUpdateFrame[:]),
		0,
	}
}

const windowUpdateMinValue = 65535 / 4 // initial window size / 4

func (p *dataFrameProcessor) Process(header frameheader.FrameHeader, _ []byte, incomplete bool) error {
	if incomplete {
		return nil
	}

	windowUpdateAcc := p.windowUpdateAcc + header.Length()
	p.windowUpdateAcc = windowUpdateAcc

	if windowUpdateAcc >= windowUpdateMinValue {
		p.currentWindowUpdateFrame[9] = byte(windowUpdateAcc >> 24)
		p.currentWindowUpdateFrame[10] = byte(windowUpdateAcc >> 16)
		p.currentWindowUpdateFrame[11] = byte(windowUpdateAcc >> 8)
		p.currentWindowUpdateFrame[12] = byte(windowUpdateAcc)

		p.outFramesChan <- p.currentWindowUpdateFrame
		p.currentWindowUpdateFrame, p.nextWindowUpdateFrame = p.nextWindowUpdateFrame, p.currentWindowUpdateFrame
		p.windowUpdateAcc = 0
	} else {
		p.windowUpdateAcc = windowUpdateAcc
	}

	if header.Flags().Has(http2.FlagDataEndStream) {
		p.streamsLimiter.Release()
		stream := p.streamsStore.GetAndDelete(header.StreamID())
		if stream != nil {
			stream.End()
		}
	}

	return nil
}

type headersFrameProcessor struct {
	streamsStore   types.StreamStore
	streamsLimiter types.StreamsLimiter

	hpackDecoder  *hpack.Decoder
	currentStream types.Stream
}

func newHeadersFrameProcessor(store types.StreamStore, limiter types.StreamsLimiter) *headersFrameProcessor {
	p := &headersFrameProcessor{
		streamsStore:   store,
		streamsLimiter: limiter,
	}
	p.hpackDecoder = hpack.NewDecoder(4096, p.OnHeader)
	return p
}

func (p *headersFrameProcessor) OnHeader(f hpack.HeaderField) {
	p.currentStream.OnHeader(f.Name, f.Value)
}

func (p *headersFrameProcessor) Process(header frameheader.FrameHeader, payload []byte, incomplete bool) error {
	streamID := header.StreamID()
	stream := p.streamsStore.Get(streamID)
	p.currentStream = stream
	p.hpackDecoder.SetEmitEnabled(stream != nil)

	_, err := p.hpackDecoder.Write(payload)
	if err != nil {
		return fmt.Errorf("hpack decoding: %w", err)
	}

	if incomplete {
		return nil
	}

	if header.Flags().Has(http2.FlagHeadersEndStream) && stream != nil {
		stream.End()
		p.streamsStore.Delete(streamID)
		p.streamsLimiter.Release()
	}
	return nil
}

type rstStreamFrameProcessor struct {
	streamsStore   types.StreamStore
	streamsLimiter types.StreamsLimiter
	errCode        uint32
}

func newRSTStreamFrameProcessor(
	streamsStore types.StreamStore,
	streamsLimiter types.StreamsLimiter,
) *rstStreamFrameProcessor {
	return &rstStreamFrameProcessor{streamsStore, streamsLimiter, 0}
}

func (p *rstStreamFrameProcessor) Process(header frameheader.FrameHeader, payload []byte, incomplete bool) error {
	for _, b := range payload {
		p.errCode = (p.errCode << 8) | uint32(b)
	}
	if incomplete {
		return nil
	}

	errCode := http2.ErrCode(p.errCode)

	streamID := header.StreamID()
	p.streamsLimiter.Release()
	stream := p.streamsStore.GetAndDelete(streamID)
	if stream != nil {
		stream.RSTStream(errCode)
		stream.End()
	}
	return nil
}

type windowUpdateFrameProcessor struct {
	fcConn       types.FlowControl
	streamsStore types.StreamStore
	increment    uint32
}

func newWindowUpdateFrameProcessor(
	streamsStore types.StreamStore, fcConn types.FlowControl,
) *windowUpdateFrameProcessor {
	return &windowUpdateFrameProcessor{fcConn, streamsStore, 0}
}

func (p *windowUpdateFrameProcessor) Process(header frameheader.FrameHeader, payload []byte, incomplete bool) error {
	for _, b := range payload {
		p.increment = (p.increment << 8) | uint32(b)
	}
	if incomplete {
		return nil
	}

	streamID := header.StreamID()

	var fc types.FlowControl
	if streamID == 0 {
		fc = p.fcConn
	} else {
		stream := p.streamsStore.Get(streamID)
		if stream == nil {
			return nil
		}
		fc = stream.FC()
	}
	fc.Add(p.increment)

	p.increment = 0
	return nil
}

type goAwayFrameProcessor struct {
	errCode      uint32
	lastStreamID uint32
	debugData    []byte
	index        int
}

func newGoAwayFrameProcessor() *goAwayFrameProcessor {
	return &goAwayFrameProcessor{}
}

func (p *goAwayFrameProcessor) Process(_ frameheader.FrameHeader, payload []byte, incomplete bool) error {
	maxIndex := p.index + len(payload)
	for ; p.index < min(4, maxIndex); p.index++ {
		b := payload[0]
		payload = payload[1:]
		p.lastStreamID = (p.lastStreamID << 8) | uint32(b)
	}

	for ; p.index < min(8, maxIndex); p.index++ {
		b := payload[0]
		payload = payload[1:]
		p.errCode = (p.errCode << 8) | uint32(b)
	}

	p.debugData = append(p.debugData, payload...)

	if incomplete {
		return nil
	}

	err := GoAwayError{
		Code:         http2.ErrCode(p.errCode),
		LastStreamID: p.lastStreamID,
		DebugData:    bytes.Clone(p.debugData),
	}

	p.index = 0
	p.errCode = 0
	p.lastStreamID = 0
	p.debugData = p.debugData[:0]
	return err
}

type settingsProcessor struct{}

func (p settingsProcessor) Process(
	header frameheader.FrameHeader,
	_ []byte,
	incomplete bool,
) error {
	if incomplete {
		return nil
	}

	if !header.Flags().Has(http2.FlagSettingsAck) {
		return errors.New("update settings in runtime not supported")
	}
	return nil
}
