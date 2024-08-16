package reciever

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/ozontech/framer/frameheader"
	"github.com/ozontech/framer/loader/types"
)

type framer struct {
	*http2.Framer
	W *bytes.Buffer
	R *bytes.Buffer
}

func newFramer() *framer {
	bufW := bytes.NewBuffer(nil)
	bufR := bytes.NewBuffer(nil)
	return &framer{
		http2.NewFramer(bufW, bufR),
		bufW, bufR,
	}
}

func unmarshalFrame(b []byte) http2.Frame {
	framer := newFramer()
	framer.R.Write(b)
	f, err := framer.ReadFrame()
	if err != nil {
		panic(fmt.Errorf("broken frame: %w", err))
	}
	return f
}

func setupProcessor(tb testing.TB, tps []FrameTypeProcessor) chan<- []byte {
	tb.Helper()

	bytesChan := make(chan []byte)
	go func() {
		err := (&Processor{new(Framer), tps}).Run(bytesChan)
		if err != nil {
			tb.Fatal(err.Error())
		}
	}()
	return bytesChan
}

func TestProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	tpData := &FrameTypeProcessorMock{}
	tpPing := &FrameTypeProcessorMock{}
	bytesChan := setupProcessor(t, []FrameTypeProcessor{
		http2.FrameData: tpData,
		http2.FramePing: tpPing,
	})
	defer close(bytesChan)

	pingPayload := [8]byte{8, 7, 6, 5, 4, 3, 2, 1}
	framer := newFramer()
	a.NoError(framer.WritePing(false, pingPayload))
	b := framer.W.Bytes()

	tpPing.ProcessFunc = func(
		header frameheader.FrameHeader,
		payload []byte,
		incomplete bool,
	) error {
		return nil
	}
	bytesChan <- b[:len(b)-1]
	bytesChan <- b[len(b)-1:]
	for {
		<-time.After(time.Millisecond)
		if len(tpPing.ProcessCalls()) == 2 {
			break
		}
	}
	a.Empty(tpData.ProcessCalls())
	calls := tpPing.ProcessCalls()
	a.True(calls[0].Incomplete)
	a.Equal(pingPayload[:len(pingPayload)-1], calls[0].Payload)
	a.False(calls[1].Incomplete)
	a.Equal(pingPayload[len(pingPayload)-1:], calls[1].Payload)
}

func TestPingFrameProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	priorityFramesChan := make(chan []byte, 1)
	fp := newPingFrameProcessor(priorityFramesChan)

	pingPayload := [8]byte{8, 7, 6, 5, 4, 3, 2, 1}
	framer := newFramer()
	a.NoError(framer.WritePing(false, pingPayload))
	b := framer.W.Bytes()

	a.NoError(fp.Process(frameheader.FrameHeader(b), pingPayload[:7], true))
	select {
	case <-priorityFramesChan:
		t.Fail()
	default:
	}
	a.NoError(fp.Process(frameheader.FrameHeader(b), pingPayload[7:], false))
	select {
	case frame := <-priorityFramesChan:
		f := unmarshalFrame(frame)
		pingFrame := f.(*http2.PingFrame)
		a.Equal(pingPayload, pingFrame.Data)
		a.Equal(http2.FramePing, pingFrame.Type)
		a.Equal(http2.FlagPingAck, pingFrame.Flags)
		a.Equal(uint32(8), pingFrame.Length)
		a.Equal(uint32(0), pingFrame.StreamID)
	default:
		t.Fail()
	}
}

func TestDataFrameProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	// do nothing on incomplete
	{
		fp := newDataFrameProcessor(nil, nil, nil)
		a.NoError(fp.Process(frameheader.FrameHeader{}, nil, true))
	}

	// publish window update frame on acummulating more than trigger length
	{
		priorityFramesChan := make(chan []byte, 1)
		fp := newDataFrameProcessor(priorityFramesChan, nil, nil)
		fh := frameheader.NewFrameHeader()
		const streamID uint32 = 123
		fh.SetStreamID(streamID)
		const trigger = windowUpdateMinValue
		fh.SetLength(trigger - 1)
		a.NoError(fp.Process(fh, nil, false))
		select {
		case <-priorityFramesChan:
			t.Error("priorityFramesChan must not resolve")
		default:
		}
		fh.SetLength(1)
		a.NoError(fp.Process(fh, nil, false))
		frame := unmarshalFrame(<-priorityFramesChan)
		wuf := frame.(*http2.WindowUpdateFrame)

		a.Equal(http2.FrameWindowUpdate, wuf.Type)
		a.Equal(http2.Flags(0), wuf.Flags)
		a.Equal(uint32(4), wuf.Length)
		a.Equal(uint32(0), wuf.StreamID)
		a.Equal(uint32(trigger), wuf.Increment)

		// check it don't trigger anymore
		fh.SetLength(1)
		a.NoError(fp.Process(fh, nil, false))
		select {
		case <-priorityFramesChan:
			t.Error("priorityFramesChan must not resolve")
		default:
		}
	}

	// release stream on end flag
	{
		const streamID uint32 = 184921
		stream := &StreamMock{
			EndFunc: func() {},
		}
		streams := &StreamStoreMock{
			GetAndDeleteFunc: func(v uint32) types.Stream {
				a.Equal(streamID, v)
				return stream
			},
		}
		streamsLimiter := &StreamsLimiterMock{
			ReleaseFunc: func() {},
		}
		fp := newDataFrameProcessor(nil, streams, streamsLimiter)
		fh := frameheader.NewFrameHeader()
		fh.SetStreamID(streamID)
		fh.SetFlags(http2.FlagDataEndStream)
		a.NoError(fp.Process(fh, nil, false))
		a.Len(streams.GetAndDeleteCalls(), 1)
		a.Len(stream.EndCalls(), 1)
	}
}

func TestHeadersFrameProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	// calls stream.OnHeader for each header
	{
		const streamID uint32 = 21123

		stream := &StreamMock{
			OnHeaderFunc: func(k, v string) {},
		}
		fp := newHeadersFrameProcessor(&StreamStoreMock{
			GetFunc: func(s uint32) types.Stream {
				a.Equal(streamID, s)
				return stream
			},
		}, &StreamsLimiterMock{
			ReleaseFunc: func() {},
		})

		b := bytes.NewBuffer(nil)
		e := hpack.NewEncoder(b)
		type call = struct {
			Name  string
			Value string
		}
		var expectedCalls []call
		headers := []hpack.HeaderField{
			{Name: ":method", Value: "POST"},
			{Name: "grpc-status", Value: "this is grpc status"},
			{Name: "grpc-message", Value: "this is grpc message"},
		}
		for _, h := range headers {
			expectedCalls = append(expectedCalls, call{
				Name:  h.Name,
				Value: h.Value,
			})
			a.NoError(e.WriteField(h))
		}
		fh := frameheader.NewFrameHeader()
		fh.SetStreamID(streamID)
		a.NoError(fp.Process(fh, b.Bytes(), true))
		a.Equal(stream.OnHeaderCalls(), expectedCalls)
	}

	// ends and delete stream on EndStreamFlag
	{
		const streamID uint32 = 21123
		stream := &StreamMock{
			EndFunc: func() {},
		}
		fp := newHeadersFrameProcessor(&StreamStoreMock{
			GetFunc: func(s uint32) types.Stream {
				a.Equal(streamID, s)
				return stream
			},
			DeleteFunc: func(s uint32) { a.Equal(streamID, s) },
		}, &StreamsLimiterMock{
			ReleaseFunc: func() {},
		})

		fh := frameheader.NewFrameHeader()
		fh.SetStreamID(streamID)
		fh.SetFlags(http2.FlagHeadersEndStream)
		a.NoError(fp.Process(fh, nil, false))
		a.Len(stream.EndCalls(), 1)
	}
}

func TestRSTStreamFrameProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	// returns RSTStreamError on rst stream frame with zero stream id
	const streamID uint32 = 21123
	const errCode = http2.ErrCodeInternal
	stream := &StreamMock{
		RSTStreamFunc: func(ec http2.ErrCode) {
			a.Equal(errCode, ec)
		},
		EndFunc: func() {},
	}
	streams := &StreamStoreMock{
		GetAndDeleteFunc: func(s uint32) types.Stream {
			a.Equal(streamID, s)
			return stream
		},
	}
	fp := newRSTStreamFrameProcessor(streams, &StreamsLimiterMock{
		ReleaseFunc: func() {},
	})

	framer := newFramer()
	a.NoError(framer.WriteRSTStream(streamID, errCode))

	header := frameheader.FrameHeader(framer.W.Next(9))
	a.NoError(fp.Process(header, framer.W.Next(1), true))
	a.NoError(fp.Process(header, framer.W.Bytes(), false))

	a.Len(streams.GetAndDeleteCalls(), 1)
	a.Len(stream.EndCalls(), 1)
	a.Len(stream.RSTStreamCalls(), 1)
}

func TestWindowUpdateFrameProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	// increment conn window when streamID = 0
	{
		const streamID uint32 = 0
		const increment uint32 = 16543
		fcConn := &FlowControlMock{
			AddFunc: func(v uint32) {
				a.Equal(increment, v)
			},
		}
		fp := newWindowUpdateFrameProcessor(nil, fcConn)

		framer := newFramer()
		a.NoError(framer.WriteWindowUpdate(streamID, increment))

		header := frameheader.FrameHeader(framer.W.Next(9))
		a.NoError(fp.Process(header, framer.W.Next(1), true))
		a.NoError(fp.Process(header, framer.W.Bytes(), false))

		a.Len(fcConn.AddCalls(), 1)
	}

	// increment stream window when streamID != 0
	{
		const streamID uint32 = 123
		const increment uint32 = 16543
		fcStream := &FlowControlMock{
			AddFunc: func(v uint32) {
				a.Equal(increment, v)
			},
		}
		stream := &StreamMock{
			FCFunc: func() types.FlowControl {
				return fcStream
			},
			EndFunc: func() {},
		}
		streams := &StreamStoreMock{
			GetFunc: func(s uint32) types.Stream {
				a.Equal(streamID, s)
				return stream
			},
		}
		fp := newWindowUpdateFrameProcessor(streams, nil)

		framer := newFramer()
		a.NoError(framer.WriteWindowUpdate(streamID, increment))

		header := frameheader.FrameHeader(framer.W.Next(9))
		a.NoError(fp.Process(header, framer.W.Next(1), true))
		a.NoError(fp.Process(header, framer.W.Bytes(), false))
		a.Len(fcStream.AddCalls(), 1)
	}

	// don't panic if stream not found
	{
		const streamID uint32 = 123
		const increment uint32 = 16543
		streams := &StreamStoreMock{
			GetFunc: func(s uint32) types.Stream {
				a.Equal(streamID, s)
				return nil
			},
		}
		fp := newWindowUpdateFrameProcessor(streams, nil)

		framer := newFramer()
		a.NoError(framer.WriteWindowUpdate(streamID, increment))

		header := frameheader.FrameHeader(framer.W.Next(9))
		a.NoError(fp.Process(header, framer.W.Next(1), true))
		a.NoError(fp.Process(header, framer.W.Bytes(), false))
		a.Len(streams.GetCalls(), 1)
	}
}

func TestGoAwayProcessor(t *testing.T) {
	t.Parallel()
	a := assert.New(t)

	// decodes and return error
	{
		const streamID uint32 = 192
		const code = http2.ErrCodeInternal
		debugData := []byte("this is debug data")

		framer := newFramer()
		a.NoError(framer.WriteGoAway(streamID, code, debugData))

		fp := newGoAwayFrameProcessor()
		header := frameheader.FrameHeader(framer.W.Next(9))
		a.NoError(fp.Process(header, framer.W.Next(1), true))
		a.Equal(
			GoAwayError{
				Code:         code,
				LastStreamID: streamID,
				DebugData:    debugData,
			},
			fp.Process(header, framer.W.Bytes(), false),
		)
	}
}
