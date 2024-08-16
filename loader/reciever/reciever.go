package reciever

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/loader/types"
)

type GoAwayError struct {
	Code         http2.ErrCode
	LastStreamID uint32
	DebugData    []byte
}

func (e GoAwayError) Error() string {
	return "go away (" + e.Code.String() + "): " + string(e.DebugData)
}

type RSTStreamError struct {
	Code http2.ErrCode
}

func (e RSTStreamError) Error() string {
	return "rst stream: " + e.Code.String()
}

type Reciever struct {
	conn      net.Conn
	buf1      []byte
	buf2      []byte
	processor *Processor
}

func NewReciever(
	conn net.Conn,
	fcConn types.FlowControl,
	priorityFramesChan chan<- []byte,
	streams types.Streams,
) *Reciever {
	return &Reciever{
		conn,
		make([]byte, consts.RecieveBufferSize),
		make([]byte, consts.RecieveBufferSize),
		NewDefaultProcessor(streams.Store, streams.Limiter, fcConn, priorityFramesChan),
	}
}

func (r *Reciever) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	ch := make(chan []byte)
	g.Go(func() error {
		return r.processor.Run(ch)
	})
	g.Go(func() error {
		defer close(ch)
		for ctx.Err() == nil {
			err := r.read(ctx, ch, r.buf1)
			if err != nil {
				return err
			}
			err = r.read(ctx, ch, r.buf2)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return g.Wait()
}

func (r *Reciever) read(ctx context.Context, ch chan<- []byte, b []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	n, err := r.conn.Read(b)
	if err != nil {
		return fmt.Errorf("reading error: %w", err)
	}
	b = b[:n]

	select {
	case ch <- b:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

//nolint:unused
func (r *Reciever) readExpiremental(ctx context.Context, ch chan<- []byte, b []byte) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := r.conn.SetReadDeadline(time.Now().Add(consts.RecieveBatchTimeout))
		if err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}

		n, err := io.ReadFull(r.conn, b)
		if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("reading error: %w", err)
		}
		if n != 0 {
			b = b[:n]
			break
		}
	}

	select {
	case ch <- b:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
