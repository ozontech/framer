package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/ozontech/framer/benchmarks/dumb-server/pb"
	"github.com/ozontech/framer/frameheader"
	"github.com/ozontech/framer/loader/flowcontrol"
	"github.com/ozontech/framer/loader/reciever"
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var commandsPool *pool.SlicePool[*command]

const (
	grpcListenAddr       = ":9090"
	reflectionListenAddr = ":9091"
)

func main() {
	commandsPool = pool.NewSlicePoolSize[*command](10240)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return run(ctx) })
	g.Go(func() error { return runReflection(ctx) })
	go func() {
		//nolint:errcheck,gosec
		http.ListenAndServe(":8080", nil)
	}()

	err := g.Wait()
	if err != nil {
		fmt.Println("server exited: " + err.Error())
		os.Exit(1)
	}
}

func runReflection(ctx context.Context) error {
	//nolint:gosec
	l, err := net.Listen("tcp", reflectionListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s := grpc.NewServer()
	pb.RegisterTestApiServer(s, nil)
	reflection.Register(s)

	go func() {
		<-ctx.Done()
		s.GracefulStop()
	}()

	return s.Serve(l)
}

type respModifier struct {
	headerStreamIDindx  int
	trailerStreamIDindx int
	dataStreamIDindx    int
}

func (r respModifier) SetStreamID(data []byte, streamID [4]byte) {
	data[r.headerStreamIDindx] = streamID[0]
	data[r.headerStreamIDindx+1] = streamID[1]
	data[r.headerStreamIDindx+2] = streamID[2]
	data[r.headerStreamIDindx+3] = streamID[3]

	data[r.trailerStreamIDindx] = streamID[0]
	data[r.trailerStreamIDindx+1] = streamID[1]
	data[r.trailerStreamIDindx+2] = streamID[2]
	data[r.trailerStreamIDindx+3] = streamID[3]

	data[r.dataStreamIDindx] = streamID[0]
	data[r.dataStreamIDindx+1] = streamID[1]
	data[r.dataStreamIDindx+2] = streamID[2]
	data[r.dataStreamIDindx+3] = streamID[3]
}

// создаем пустой grpc-ответ
func newResps() (resp1 []byte, resp1Mod respModifier, resp2 []byte, resp2Mod respModifier) {
	headerBuf := bytes.NewBuffer(nil)
	enc := hpack.NewEncoder(headerBuf)

	makeResp := func() (resp []byte, respMod respModifier) {
		headerBuf.Reset()

		enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})                   //nolint:errcheck // writing to buffer is safe
		enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/grpc"}) //nolint:errcheck // writing to buffer is safe
		frameHeader := frameheader.NewFrameHeader()
		frameHeader.Fill(
			headerBuf.Len(),
			http2.FrameHeaders,
			http2.FlagHeadersEndHeaders,
			0,
		)
		respMod.headerStreamIDindx = len(resp) + 5
		resp = append(resp, frameHeader...)
		resp = append(resp, headerBuf.Bytes()...)

		msgPrefix := make([]byte, 5) // не кодированный ответ 0 длины = любое сообщение со всеми пустыми полями
		dataLen := len(msgPrefix)

		respMod.dataStreamIDindx = len(resp) + 5
		frameHeader.Fill(
			dataLen,
			http2.FrameData,
			0,
			0,
		)
		resp = append(resp, frameHeader...)
		resp = append(resp, msgPrefix...)

		headerBuf.Reset()
		//nolint:errcheck // writing to buffer is safe
		enc.WriteField(hpack.HeaderField{Name: "grpc-status", Value: "0"})
		frameHeader.Fill(
			headerBuf.Len(),
			http2.FrameHeaders,
			http2.FlagHeadersEndHeaders|http2.FlagHeadersEndStream,
			0,
		)
		respMod.trailerStreamIDindx = len(resp) + 5
		resp = append(resp, frameHeader...)
		resp = append(resp, headerBuf.Bytes()...)
		return resp, respMod
	}

	resp1, resp1Mod = makeResp()
	resp2, resp2Mod = makeResp()
	return
}

var (
	bytesIN           atomic.Uint64
	scheduledBytesOUT atomic.Uint64
	bytesOUT          atomic.Uint64

	requestsIN  atomic.Uint64
	requestsOUT atomic.Uint64
)

func run(ctx context.Context) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	//nolint:gosec
	l, err := net.Listen("tcp", grpcListenAddr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	defer func() {
		closeErr := l.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	go func() {
		<-ctx.Done()
		l.Close()
	}()

	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
			case <-t.C:
				if bytesIN.Load() > 0 {
					println(
						"bytesIN:", humanize.Bytes(bytesIN.Swap(0)),
						"scheduledBytesOUT:", humanize.Bytes(scheduledBytesOUT.Swap(0)),
						"bytesOUT:", humanize.Bytes(bytesOUT.Swap(0)),
						"requestsIN:", requestsIN.Swap(0),
						"requestsOUT:", requestsOUT.Swap(0),
					)
				}
			}
		}
	}()

	var i int
	for {
		conn, err := l.Accept()
		if err != nil {
			return fmt.Errorf("accept: %w", err)
		}

		i++
		go func(conn net.Conn, i int) {
			err := handleConn(conn, i)
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("handle done without error")
			}
		}(conn, i)
	}
}

const (
	maxFramePayloadLen int = 16384 // Максимальная длина пейлоада фрейма в grpc. У http2 ограничение больше.
	prefaceLen             = len(http2.ClientPreface)
)

type settings struct {
	MaxConcurrentStreams uint32
	InitialWindowSize    uint32
}

type commandType int

const (
	commandTypeResp commandType = iota
	commandTypePing
)

type command struct {
	commandType  commandType
	respStreamID [4]byte
	pingPayload  [8]byte
}

//nolint:unused
func handleNoop(conn net.Conn, _ int) (err error) {
	defer conn.Close()

	buf := make([]byte, maxFramePayloadLen)
	_, err = configureConn(conn, buf)
	if err != nil {
		return fmt.Errorf("conn configuration: %w", err)
	}

	for {
		_, err := conn.Read(buf)
		if err != nil {
			return fmt.Errorf("read %w", err)
		}
	}
}

func handleConn(conn net.Conn, i int) (err error) {
	defer conn.Close()
	defer println("handle done")

	buf1 := make([]byte, maxFramePayloadLen)
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("conn closing: %w", closeErr))
		}
	}()

	s, err := configureConn(conn, buf1)
	if err != nil {
		return fmt.Errorf("conn configuration: %w", err)
	}
	fc := flowcontrol.NewFlowControl(s.InitialWindowSize)

	cmds := make(chan *command, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		defer println("write loop done")
		defer cancel()
		err := writeLoop(ctx, conn, cmds, fc, i)
		if err != nil {
			return fmt.Errorf("%d writeLoop error: %w", i, err)
		}
		return nil
	})

	ch := make(chan []byte)
	endStreamProcessor := &endStreamProcessor{cmds}
	processor := reciever.NewProcessor([]reciever.FrameTypeProcessor{
		http2.FrameData:         endStreamProcessor,
		http2.FrameHeaders:      endStreamProcessor,
		http2.FramePing:         &pingProcessor{cmds},
		http2.FrameContinuation: endStreamProcessor,

		http2.FrameRSTStream:    noopFrameProcessor{},
		http2.FrameGoAway:       noopFrameProcessor{},
		http2.FrameWindowUpdate: noopFrameProcessor{},

		// http.FramePriority not supported
		http2.FrameSettings: settingsProcessor{},
		// http.FramePushPromise not supported
	})
	g.Go(func() error {
		defer println("processor done")
		return processor.Run(ch)
	})

	// readfn := func(b []byte) (int, error) {
	// 	for {
	// 		err := conn.SetReadDeadline(time.Now().Add(time.Millisecond))
	// 		if err != nil {
	// 			return 0, fmt.Errorf("set read deadline: %w", err)
	// 		}

	// 		n, err := io.ReadFull(conn, b)
	// 		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
	// 			return n, fmt.Errorf("reading error: %w", err)
	// 		}
	// 		bytesIN.Add(uint64(n))
	// 		if n != 0 {
	// 			return n, nil
	// 		}
	// 	}
	// }
	readfn := func(b []byte) (int, error) {
		n, err := conn.Read(b)
		bytesIN.Add(uint64(n))
		return n, err
	}

	buf2 := bytes.Clone(buf1)
	g.Go(func() error {
		defer close(ch)
		defer cancel()
		for {
			n, err := readfn(buf1)
			if err != nil {
				return fmt.Errorf("read %w", err)
			}

			select {
			case ch <- buf1[:n]:
			case <-ctx.Done():
				return ctx.Err()
			}

			n, err = readfn(buf2)
			if err != nil {
				return fmt.Errorf("read %w", err)
			}

			select {
			case ch <- buf2[:n]:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	})

	return g.Wait()
}

func writeLoop(ctx context.Context, conn io.Writer, commands <-chan *command, _ types.FlowControl, _ int) (err error) {
	r1Original, r1mod, r2Original, r2mod := newResps()
	select {
	case cmd := <-commands:
		switch cmd.commandType {
		case commandTypeResp:
			r1mod.SetStreamID(r1Original, cmd.respStreamID)
			_, err := conn.Write(r1Original)
			if err != nil {
				return err
			}
		case commandTypePing:
			err := http2.NewFramer(conn, nil).WritePing(true, cmd.pingPayload)
			if err != nil {
				return err
			}
		}
	case <-ctx.Done():
		return nil
	}

	const limit = 1024

	type t struct {
		bufSrc    [limit][]byte
		requests  [limit][]byte
		pingFrame []byte
	}

	var t1, t2 t
	for i := 0; i < limit; i++ {
		t1.requests[i] = bytes.Clone(r2Original)
		t2.requests[i] = bytes.Clone(r2Original)
	}

	{
		pingFrame := make([]byte, 9+8)
		pingHeader := frameheader.FrameHeader(pingFrame)
		pingHeader.SetType(http2.FramePing)
		pingHeader.SetLength(8)
		pingHeader.SetFlags(http2.FlagPingAck)

		t1.pingFrame = pingFrame
		t2.pingFrame = bytes.Clone(pingFrame)
	}

	writeChan := make(chan net.Buffers) // важно чтобы канал был небуферизованный!
	go func() {
		ticker := time.NewTicker(time.Millisecond)
		defer close(writeChan)
		defer ticker.Stop()

		nextT, t := &t1, &t2
		for {
			t, nextT = nextT, t

			buf := t.bufSrc[:1]
			var writeBuf net.Buffers
			for {
				select {
				case cmd := <-commands:
					switch cmd.commandType {
					case commandTypeResp:
						r2 := t.requests[len(buf)]
						r2mod.SetStreamID(r2, cmd.respStreamID)
						buf = append(buf, r2)
						scheduledBytesOUT.Add(uint64(len(r2)))

						if len(buf) == limit {
							writeBuf = buf[1:] // НЕ пишем резервную позицию
						}
					case commandTypePing:
						copy(t.pingFrame[9:], cmd.pingPayload[:])
						buf[0] = t.pingFrame
						writeBuf = buf // пишем резервную позицию
					}
					commandsPool.Release(cmd)
				case <-ticker.C:
					writeBuf = buf[1:] // НЕ пишем резервную позицию
				case <-ctx.Done():
					return
				}

				if writeBuf != nil {
					writeChan <- writeBuf
					break
				}
			}
		}
	}()

	for bufs := range writeChan {
		l := uint64(len(bufs))
		cmdsBefore := len(commands)
		writeStart := time.Now()
		_, err = bufs.WriteTo(conn)
		if err != nil {
			return fmt.Errorf("buffs writing: %w", err)
		}
		cmdsAfter := len(commands)

		since := time.Since(writeStart)
		if since > time.Millisecond*10 && false {
			println(since.String(), cmdsBefore, cmdsAfter)
		}
		bytesOUT.Add(l * uint64(len(r2Original)))
		requestsOUT.Add(l)
	}
	return nil
}

func configureConn(conn io.ReadWriter, buf []byte) (settings, error) {
	s := settings{InitialWindowSize: 65535}

	preface := buf[:prefaceLen]
	_, err := conn.Read(preface)
	if err != nil {
		return s, fmt.Errorf("reading preface: %w", err)
	}
	if !bytes.Equal(preface, []byte(http2.ClientPreface)) {
		return s, fmt.Errorf("got bad preface: %s", preface)
	}

	framer := http2.NewFramer(conn, conn)
	err = framer.WriteSettings(http2.Setting{
		ID:  http2.SettingInitialWindowSize,
		Val: math.MaxUint32 & 0x7fffffff, // mask off high reserved bit
	})
	if err != nil {
		return s, fmt.Errorf("write settings frame: %w", err)
	}

	frame, err := framer.ReadFrame()
	if err != nil {
		return s, fmt.Errorf("read settings frame: %w", err)
	}

	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return s, fmt.Errorf("first frame from other end is not settings, got %T", frame)
	}

	if val, ok := sf.Value(http2.SettingInitialWindowSize); ok {
		s.InitialWindowSize = val
	}
	if val, ok := sf.Value(http2.SettingMaxConcurrentStreams); ok {
		s.MaxConcurrentStreams = val
	}

	err = framer.WriteSettingsAck()
	if err != nil {
		return s, fmt.Errorf("writing settings ack: %w", err)
	}

	// у h2load на валидное значение происходит переполнение буффера с отвалом соедиенинения, поэтому: - 65_535
	const increment = math.MaxUint32&0x7fffffff - 65_535
	err = framer.WriteWindowUpdate(0, increment)
	if err != nil {
		return s, fmt.Errorf("increasing conn fc window: %w", err)
	}

	return s, nil
}

type noopFrameProcessor struct{}

func (p noopFrameProcessor) Process(
	_ frameheader.FrameHeader,
	_ []byte,
	_ bool,
) error {
	return nil
}

type endStreamProcessor struct {
	respChan chan<- *command
}

func (p *endStreamProcessor) Process(
	header frameheader.FrameHeader,
	_ []byte,
	incomplete bool,
) error {
	if incomplete || !header.Flags().Has(http2.FlagDataEndStream) {
		return nil
	}
	requestsIN.Add(1)

	c, ok := commandsPool.Acquire()
	if !ok {
		c = new(command)
	}
	c.commandType = commandTypeResp
	copy(c.respStreamID[:], header[5:])
	p.respChan <- c

	return nil
}

type pingProcessor struct {
	respChan chan<- *command
}

func (p *pingProcessor) Process(
	header frameheader.FrameHeader,
	payload []byte,
	incomplete bool,
) error {
	if incomplete || !header.Flags().Has(http2.FlagPingAck) {
		return nil
	}

	c, ok := commandsPool.Acquire()
	if !ok {
		c = new(command)
	}
	c.commandType = commandTypePing
	copy(c.pingPayload[:], payload)
	p.respChan <- c

	return nil
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
