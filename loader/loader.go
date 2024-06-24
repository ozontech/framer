package loader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"

	"github.com/ozontech/framer/consts"
	fc "github.com/ozontech/framer/loader/flowcontrol"
	"github.com/ozontech/framer/loader/reciever"
	"github.com/ozontech/framer/loader/sender"
	streamsPool "github.com/ozontech/framer/loader/streams/pool"
	streamsStore "github.com/ozontech/framer/loader/streams/store"
	"github.com/ozontech/framer/loader/types"
	hpackwrapper "github.com/ozontech/framer/utils/hpack_wrapper"
)

var clientPreface = []byte(http2.ClientPreface)

type TimeoutQueue interface {
	Add(streamID uint32)
	Next() (uint32, bool)
	Close()
}

type Loader struct {
	conn         net.Conn
	streamsPool  *streamsPool.StreamsPool
	streamsStore types.StreamStore
	timeoutQueue TimeoutQueue

	sender   *sender.Sender
	reciever *reciever.Reciever
	loaderID int32

	log *zap.Logger
}

func NewLoader(
	conn net.Conn,
	reporter types.Reporter,
	timeout time.Duration,
	log *zap.Logger,
) (*Loader, error) {
	conf := loaderConfig{timeout: timeout}

	err := conn.SetDeadline(time.Now().Add(conf.timeout))
	if err != nil {
		return nil, fmt.Errorf("set conn deadline: %w", err)
	}

	err = setupHTTP2(conn, conn, &conf)
	if err != nil {
		return nil, err
	}

	return newLoader(conn, reporter, conf, log), nil
}

type loaderConfig struct {
	timeout              time.Duration
	maxConcurrentStreams uint32
	initialWindowSize    uint32
	maxDymanicTableSize  uint32
	maxFrameSize         uint32
}

var i int32

func newLoader(
	conn net.Conn,
	reporter types.LoaderReporter,
	conf loaderConfig,
	log *zap.Logger,
) *Loader {
	if conf.timeout == 0 {
		conf.timeout = consts.DefaultTimeout
	}
	loaderID := atomic.AddInt32(&i, 1)
	log = log.Named("loader").With(zap.Int32("loader-id", loaderID))
	log.Debug("loader created")

	streamsStore := streamsStore.NewShardedStreamsMap(16, func() types.StreamStore {
		return streamsStore.NewStreamsMap()
	})
	timeoutQueue := NewTimeoutQueue(conf.timeout)
	fcConn := fc.NewFlowControl(consts.DefaultInitialWindowSize) // для соединения (по спеке игнорирует SETTINGS_INITIAL_WINDOW_SIZE)

	var streamPoolOpts []streamsPool.Opt
	if conf.maxConcurrentStreams != 0 {
		streamPoolOpts = append(streamPoolOpts, streamsPool.WithMaxConcurrentStreams(conf.maxConcurrentStreams))
	}
	if conf.initialWindowSize != 0 {
		streamPoolOpts = append(streamPoolOpts, streamsPool.WithInitialWindowSize(conf.initialWindowSize))
	}
	streamsPool := streamsPool.NewStreamsPool(reporter, streamPoolOpts...)

	var hpackWrapperOpts []hpackwrapper.Opt
	if conf.maxDymanicTableSize != 0 {
		hpackWrapperOpts = append(hpackWrapperOpts, hpackwrapper.WithMaxDynamicTableSize(conf.maxDymanicTableSize))
	}
	hpackWrapper := hpackwrapper.NewWrapper(hpackWrapperOpts...)

	maxFrameSize := consts.DefaultMaxFrameSize
	if conf.maxFrameSize != 0 {
		maxFrameSize = int(conf.maxFrameSize)
	}

	priorityFramesCh := make(chan []byte, 1)
	return &Loader{
		conn:         conn,
		timeoutQueue: timeoutQueue,
		log:          log,
		loaderID:     loaderID,

		streamsStore: streamsStore,

		streamsPool: streamsPool,
		sender: sender.NewSender(
			conn, fcConn, priorityFramesCh, streamsPool,
			streamsStore, hpackWrapper, maxFrameSize,
		),
		reciever: reciever.NewReciever(
			conn, fcConn, priorityFramesCh, streamsStore,
		),
	}
}

func (l *Loader) Shutdown(ctx context.Context) (err error) {
	defer func() {
		l.timeoutQueue.Close()
		err = multierr.Append(err, l.conn.Close())
	}()

	if l.streamsPool.InUse() == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// на этом этапе балансировщик уже не должен отправлять
	// запросы в этот инстц
	// поэтому дожидаемся релиза всех стримов и отменяем контекст
	// тем самым закрывая соединение
	go func() {
		l.WaitResponses(ctx)
		println(l.loaderID)
		cancel()
	}()

	return l.Run(ctx)
}

func (l *Loader) Flush(context.Context) {
	l.sender.Flush()
}

func (l *Loader) WaitResponses(ctx context.Context) {
	select {
	case <-l.streamsPool.WaitAllReleased():
		l.log.Debug("all streams released")
	case <-time.After(5 * time.Second):
		println(l.loaderID)
		l.streamsStore.Each(func(stream types.Stream) {
			println(stream.ID())
		})
	case <-ctx.Done():
	}
}

func (l *Loader) Run(ctx context.Context) error {
	err := l.conn.SetDeadline(time.Time{})
	if err != nil {
		return err
	}

	defer l.log.Debug("run done")

	go func() {
		for {
			streamID, ok := l.timeoutQueue.Next()
			if !ok {
				return
			}

			s := l.streamsStore.GetAndDelete(streamID)
			if s == nil {
				continue
			}
			s.End()
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		<-ctx.Done()
		return l.conn.SetDeadline(time.Now())
	})

	g.Go(func() error {
		defer cancel()
		return l.runSender(ctx)
	})

	g.Go(func() error {
		defer cancel()
		return l.runReciever(ctx)
	})

	return g.Wait()
}

func (l *Loader) runReciever(ctx context.Context) (err error) {
	defer l.log.Debug("reader done")

	err = l.reciever.Run(ctx)
	if err == nil || ctx.Err() != nil {
		return nil
	}

	var goAwayErr reciever.GoAwayError
	if !errors.As(err, &goAwayErr) {
		return err
	}

	l.streamsStore.Each(func(s types.Stream) {
		if s.ID() > goAwayErr.LastStreamID {
			s.GoAway(goAwayErr.Code)
			s.End()
		}
	})

	if goAwayErr.Code != http2.ErrCodeNo {
		return goAwayErr
	}
	return nil
}

func (l *Loader) runSender(ctx context.Context) (err error) {
	defer func() { l.log.Debug("sender event loop done", zap.Error(err)) }()

	err = l.sender.Run(ctx)
	if err == nil || ctx.Err() != nil {
		return nil
	}
	return err
}

func (l *Loader) DoRequest(req types.Req) {
	l.sender.Send(req)
}

func setupHTTP2(r io.Reader, w io.Writer, conf *loaderConfig) error {
	// we should not check n, because Write must return error on n < len(clientPreface)
	_, err := w.Write(clientPreface)
	if err != nil {
		return fmt.Errorf("write http2 preface: %w", err)
	}

	framer := http2.NewFramer(w, r)

	// handle settings
	frame, err := framer.ReadFrame()
	if err != nil {
		return fmt.Errorf("read settings frame: %w", err)
	}

	sf, ok := frame.(*http2.SettingsFrame)
	if !ok {
		return errors.New("protocol error: first frame from server is not settings")
	}

	for i := 0; i < sf.NumSettings(); i++ {
		s := sf.Setting(i)
		switch s.ID {
		case http2.SettingInitialWindowSize:
			conf.initialWindowSize = s.Val
		case http2.SettingMaxConcurrentStreams:
			conf.maxConcurrentStreams = s.Val
		case http2.SettingHeaderTableSize:
			conf.maxDymanicTableSize = s.Val
		case http2.SettingMaxFrameSize:
			conf.maxFrameSize = s.Val
		default:
			return fmt.Errorf("got not supported setting: %s (%d)", s.ID.String(), s.Val)
		}
	}

	err = framer.WriteSettings(http2.Setting{
		ID:  http2.SettingInitialWindowSize,
		Val: math.MaxUint32 & 0x7fffffff, // mask off high reserved bit
	})
	if err != nil {
		return fmt.Errorf("write settings frame: %w", err)
	}

	err = framer.WriteSettingsAck()
	if err != nil {
		return fmt.Errorf("write settings ack: %w", err)
	}

	err = framer.WriteWindowUpdate(0, math.MaxUint32&0x7fffffff)
	if err != nil {
		return fmt.Errorf("write window update frame: %w", err)
	}

	return nil
}
