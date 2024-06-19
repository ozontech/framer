package converter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"golang.org/x/sync/errgroup"
)

type DataHolder = interface{}

type convertItem struct {
	message    DataHolder
	convertErr error
}

type Strategy interface {
	Read() (DataHolder, error)
	Decode(DataHolder) error

	Encode(DataHolder) error
	Write(DataHolder) error
}

type Processor struct {
	conf     conf
	strategy Strategy
}

func NewProcessor(strategy Strategy, opts ...Option) *Processor {
	conf := newDefaultConf() //nolint:govet
	for _, o := range opts {
		if o != nil {
			o(&conf)
		}
	}

	return &Processor{
		conf:     conf,
		strategy: strategy,
	}
}

func (p *Processor) runRead(ctx context.Context, readChans []chan convertItem) error {
	defer func() {
		for _, readChan := range readChans {
			close(readChan)
		}
	}()

	s := p.strategy
	threads := p.conf.threads

	var i int
	for {
		message, err := s.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read #%d message error: %w", i+1, err)
		}

		select {
		case readChans[i%threads] <- convertItem{message, nil}:
		case <-ctx.Done():
			return ctx.Err()
		}
		i++
	}
}

func (p *Processor) runConvert(ctx context.Context, convertChan <-chan convertItem, writeChan chan<- convertItem) error {
	defer close(writeChan)

	s := p.strategy

	for {
		select {
		case item, ok := <-convertChan:
			if !ok {
				return nil
			}

			if err := s.Decode(item.message); err != nil {
				item.convertErr = fmt.Errorf("decode error: %w", err)
			} else if err = s.Encode(item.message); err != nil {
				item.convertErr = fmt.Errorf("encode error: %w", err)
			}

			select {
			case writeChan <- item:
			case <-ctx.Done():
				return ctx.Err()
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Processor) runWrite(ctx context.Context, writeChans []chan convertItem) error {
	threads := p.conf.threads
	errWriter := p.conf.errWriter
	s := p.strategy

	var (
		i           int
		closedChans int
	)
	for {
		select {
		case item, ok := <-writeChans[i%threads]:
			if !ok {
				closedChans++
				if closedChans == threads {
					return nil
				}
				continue
			}

			i++
			if item.convertErr != nil {
				err := fmt.Errorf("message #%d: %w", i, item.convertErr)
				if p.conf.failOnConvertErrors {
					return err
				}
				_, err = errWriter.Write([]byte(err.Error() + "\n"))
				if err != nil {
					return fmt.Errorf("errWriter: %w", err)
				}
				continue
			}
			err := s.Write(item.message)
			if err != nil {
				return fmt.Errorf("write: %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (p *Processor) Process(ctx context.Context) error {
	threads := p.conf.threads
	readChans := make([]chan convertItem, threads)
	for i := range readChans {
		readChans[i] = make(chan convertItem, p.conf.threadBuffer)
	}
	writeChans := make([]chan convertItem, threads)
	for i := range readChans {
		writeChans[i] = make(chan convertItem)
	}

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() (err error) { return p.runRead(ctx, readChans) })

	for i := 0; i < threads; i++ {
		i := i
		g.Go(func() error { return p.runConvert(ctx, readChans[i], writeChans[i]) })
	}

	g.Go(func() error { return p.runWrite(ctx, writeChans) })

	return g.Wait()
}

type conf struct {
	threads             int
	threadBuffer        int
	errWriter           io.Writer
	failOnConvertErrors bool
}

func newDefaultConf() conf {
	return conf{
		threads:      runtime.GOMAXPROCS(-1),
		threadBuffer: 100,
		errWriter:    os.Stderr,
	}
}

type Option func(*conf)

func WithThreads(threads int) Option {
	return func(r *conf) {
		r.threads = threads
	}
}

func WithThreadsBuffer(threadBuffer int) Option {
	return func(c *conf) {
		c.threadBuffer = threadBuffer
	}
}

func WithErrWriter(errWriter io.Writer) Option {
	return func(c *conf) {
		c.errWriter = errWriter
	}
}

func WithFailOnConvertErrors() Option {
	return func(r *conf) {
		r.failOnConvertErrors = true
	}
}
