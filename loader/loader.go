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

	fc "github.com/ozontech/framer/loader/flowcontrol"
	"github.com/ozontech/framer/loader/reciever"
	"github.com/ozontech/framer/loader/sender"
	streamsPool "github.com/ozontech/framer/loader/streams/pool"
	streamsStore "github.com/ozontech/framer/loader/streams/store"
	"github.com/ozontech/framer/loader/types"
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

	conf Config

	sender   *sender.Sender
	reciever *reciever.Reciever
	loaderID int32

	log *zap.Logger
}

type Config struct {
	Timeout      time.Duration // таймаут для запросов
	StreamsLimit uint32        `config:"max-open-streams" validate:"min=0"`
}

const defaultTimeout = 11 * time.Second

func DefaultConfig() Config {
	return Config{
		Timeout:      defaultTimeout,
		StreamsLimit: 0,
	}
}

var i int32

func NewLoader(
	ctx context.Context,
	conn net.Conn,
	reporter types.Reporter,
	log *zap.Logger,
	conf Config,
) (*Loader, error) {
	l := newLoader(conn, reporter, log, conf)
	setupCtx, setupCancel := context.WithTimeout(ctx, conf.Timeout)
	defer setupCancel()
	return l, l.setup(setupCtx)
}

func newLoader(
	conn net.Conn,
	reporter types.LoaderReporter,
	log *zap.Logger,
	conf Config,
) *Loader {
	loaderID := atomic.AddInt32(&i, 1)
	log = log.Named("loader").With(zap.Int32("loader-id", loaderID))
	log.Debug("loader created")

	// streamsStore := streamsStore.NewStreamsNoop()
	streamsStore := streamsStore.NewShardedStreamsMap(16, func() types.StreamStore {
		return streamsStore.NewStreamsMap()
	})
	// streamsStore := NewLimitedStreams(NewStreamsSyncMap(), loaderID, conf.StreamsLimit)
	// streamsStore := NewLimitedStreams(NewStreamsMap(), loaderID, conf.StreamsLimit)
	timeoutQueue := NewTimeoutQueue(conf.Timeout)

	// TODO(pgribanov): math.MaxUint32?
	fcConn := fc.NewFlowControl(math.MaxUint32) // для соединения (не может меняться в течение жизни соединения)

	priorityFramesCh := make(chan []byte, 1)
	streamsPool := streamsPool.NewStreamsPool(reporter, 1024, conf.StreamsLimit)
	return &Loader{
		conn:         conn,
		conf:         conf,
		timeoutQueue: timeoutQueue,
		log:          log,
		loaderID:     loaderID,

		streamsStore: streamsStore,

		streamsPool: streamsPool,
		sender:      sender.NewSender(conn, fcConn, priorityFramesCh, streamsPool, streamsStore),
		reciever:    reciever.NewReciever(conn, fcConn, priorityFramesCh, streamsStore),
	}
}

func (l *Loader) setup(ctx context.Context) (err error) {
	deadline, ok := ctx.Deadline()
	if ok {
		err = l.conn.SetDeadline(deadline)
		if err != nil {
			return fmt.Errorf("set conn deadline: %w", err)
		}
	}

	connConf, err := setupHTTP2(l.conn, l.conn)
	if err != nil {
		return err
	}

	if connConf.InitialWindowSize != 0 {
		l.streamsPool.SetInitialWindowSize(connConf.InitialWindowSize)
	}
	if connConf.MaxConcurrentStreams != 0 {
		l.streamsPool.SetLimit(connConf.MaxConcurrentStreams)
	}
	return nil
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

type connConfig struct {
	InitialWindowSize    uint32
	MaxConcurrentStreams uint32
}

func setupHTTP2(r io.Reader, w io.Writer) (connConfig, error) {
	var conf connConfig

	// we should not check n, because Write must return error on n < len(clientPreface)
	_, err := w.Write(clientPreface)
	if err != nil {
		return conf, fmt.Errorf("write http2 preface: %w", err)
	}

	framer := http2.NewFramer(w, r)

	// handle settings
	{
		frame, err := framer.ReadFrame()
		if err != nil {
			return conf, fmt.Errorf("read settings frame: %w", err)
		}

		sf, ok := frame.(*http2.SettingsFrame)
		if !ok {
			return conf, errors.New("protocol error: first frame from server is not settings")
		}
		if val, ok := sf.Value(http2.SettingInitialWindowSize); ok {
			conf.InitialWindowSize = val
		}
		if val, ok := sf.Value(http2.SettingMaxConcurrentStreams); ok {
			conf.MaxConcurrentStreams = val
		}

		err = framer.WriteSettings(http2.Setting{
			ID:  http2.SettingInitialWindowSize,
			Val: math.MaxUint32 & 0x7fffffff, // mask off high reserved bit
		})
		if err != nil {
			return conf, fmt.Errorf("write settings frame: %w", err)
		}

		err = framer.WriteSettingsAck()
		if err != nil {
			return conf, fmt.Errorf("write settings ack: %w", err)
		}

		err = framer.WriteWindowUpdate(0, math.MaxUint32&0x7fffffff)
		if err != nil {
			return conf, fmt.Errorf("write window update frame: %w", err)
		}
	}

	return conf, nil
}
