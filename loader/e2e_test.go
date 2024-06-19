package loader

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"os"
	"testing"

	"github.com/ozontech/framer/datasource"
	"github.com/ozontech/framer/loader/types"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestE2E(t *testing.T) {
	t.Parallel()
	log := zaptest.NewLogger(t)
	a := assert.New(t)
	clientConn, serverConn := net.Pipe()
	l := newLoader(clientConn, nooReporter{}, log, DefaultConfig())

	requestsFile, err := os.Open("../test_files/requests")
	if err != nil {
		a.NoError(err)
	}
	dataSource := datasource.NewFileDataSource(datasource.NewCyclicReader(requestsFile))

	const reqCount = 10_000
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return l.Run(ctx) })
	g.Go(func() (err error) {
		defer func() { log.Info("scheduling done", zap.Error(err)) }()
		for i := 0; i < reqCount; i++ {
			req, err := dataSource.Fetch()
			if err != nil {
				return err
			}
			l.DoRequest(req)
		}
		l.WaitResponses(ctx)
		return nil
	})

	framer := http2.NewFramer(serverConn, serverConn)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, func(f hpack.HeaderField) {})
	framer.SetMaxReadFrameSize(256)
	respChan := make(chan uint32, 128)

	g.Go(func() (err error) {
		defer func() { log.Info("sending done", zap.Error(err)) }()
		defer cancel()

		headerBuf := bytes.NewBuffer(nil)
		enc := hpack.NewEncoder(headerBuf)
		data := make([]byte, 5)

		for streamID := range respChan {
			headerBuf.Reset()
			a.NoError(enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"}))
			a.NoError(enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/grpc"}))
			err := framer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: headerBuf.Bytes(),
				EndHeaders:    true,
			})
			if err != nil {
				return fmt.Errorf("writing headers: %w", err)
			}

			err = framer.WriteData(streamID, false, data)
			if err != nil {
				return fmt.Errorf("writing data: %w", err)
			}

			headerBuf.Reset()
			a.NoError(enc.WriteField(hpack.HeaderField{Name: "grpc-status", Value: "0"}))
			err = framer.WriteHeaders(http2.HeadersFrameParam{
				StreamID:      streamID,
				BlockFragment: headerBuf.Bytes(),
				EndHeaders:    true,
				EndStream:     true,
			})
			if err != nil {
				return fmt.Errorf("writing headers: %w", err)
			}
		}
		return nil
	})
	g.Go(func() (err error) {
		defer func() { log.Info("recieving done", zap.Error(err)) }()
		defer close(respChan)

		expectedHeaders := []hpack.HeaderField{
			{Name: ":path", Value: "/test.api.TestApi/Test"},
			{Name: ":method", Value: "POST"},
			{Name: ":scheme", Value: "http"},
			{Name: "content-type", Value: "application/grpc"},
			{Name: "te", Value: "trailers"},
			{Name: "x-my-header-key1", Value: "my-header-val1"},
			{Name: "x-my-header-key2", Value: "my-header-val2"},
		}

		b := make([]byte, 5)
		b = protowire.AppendTag(b, 1, protowire.BytesType)
		b = protowire.AppendString(b, "ping")
		binary.BigEndian.PutUint32(b[1:5], uint32(len(b)-5))

		err = framer.WriteWindowUpdate(0, math.MaxUint32&0x7fffffff)
		if err != nil {
			return fmt.Errorf("write window update frame: %w", err)
		}

		for i := 0; i < reqCount; i++ {
			var frame http2.Frame
			for i := 0; i < 2; i++ {
				frame, err = framer.ReadFrame()
				if err != nil {
					return fmt.Errorf("read frame: %w", err)
				}
				if frame.Header().Type == http2.FrameWindowUpdate {
					continue
				}
				break
			}

			headersFrame := frame.(*http2.MetaHeadersFrame)
			a.Equal(expectedHeaders, headersFrame.Fields)

			for i := 0; i < 2; i++ {
				frame, err = framer.ReadFrame()
				if err != nil {
					return fmt.Errorf("read frame: %w", err)
				}
				if frame.Header().Type == http2.FrameWindowUpdate {
					continue
				}
				break
			}
			dataFrame := frame.(*http2.DataFrame)
			a.Equal(b, dataFrame.Data())
			respChan <- dataFrame.StreamID
		}

		return nil
	})
	a.NoError(g.Wait())
}

type nooReporter struct{}

func (a nooReporter) Acquire(string) types.StreamState {
	return streamState{}
}

type streamState struct{}

func (s streamState) SetSize(int)             {}
func (s streamState) Reset(string)            {}
func (s streamState) OnHeader(string, string) {}
func (s streamState) IoError(error)           {}
func (s streamState) RSTStream(http2.ErrCode) {}
func (s streamState) GoAway(http2.ErrCode)    {}
func (s streamState) End()                    {}
