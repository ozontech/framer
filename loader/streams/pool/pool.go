package pool

import (
	"sync"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/loader/flowcontrol"
	"github.com/ozontech/framer/loader/types"
)

type StreamsPool struct {
	reporter types.LoaderReporter

	cond *sync.Cond
	pool []*streamImpl

	inUse             uint32
	initialWindowSize uint32
}

func NewStreamsPool(reporter types.LoaderReporter, opts ...Opt) *StreamsPool {
	p := &StreamsPool{
		reporter:          reporter,
		cond:              sync.NewCond(&sync.Mutex{}),
		pool:              make([]*streamImpl, 0, 1024),
		initialWindowSize: consts.DefaultInitialWindowSize,
	}
	for _, o := range opts {
		o.apply(p)
	}
	return p
}

func (p *StreamsPool) Acquire(streamID uint32, tag string) types.Stream {
	var stream *streamImpl
	p.cond.L.Lock()
	p.inUse++
	l := len(p.pool)
	if l > 0 {
		last := l - 1
		stream = p.pool[last]
		p.pool = p.pool[:last]
		p.cond.L.Unlock()
	} else {
		p.cond.L.Unlock()
		stream = &streamImpl{pool: p, fc: flowcontrol.NewFlowControl(p.initialWindowSize)}
	}

	stream.streamID = streamID
	stream.fc.Reset(p.initialWindowSize)
	stream.StreamState = p.reporter.Acquire(tag, streamID)

	return stream
}

func (p *StreamsPool) release(stream *streamImpl) {
	p.cond.L.Lock()
	defer p.cond.Signal()
	defer p.cond.L.Unlock()

	p.pool = append(p.pool, stream)
	p.inUse--
}

func (p *StreamsPool) InUse() uint32 {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	return p.inUse
}

func (p *StreamsPool) WaitAllReleased() <-chan struct{} {
	ch := make(chan struct{})

	go func() {
		p.cond.L.Lock()
		defer p.cond.L.Unlock()

		for p.inUse != 0 {
			p.cond.Wait()
		}

		close(ch)
	}()

	return ch
}

type streamImpl struct {
	pool     *StreamsPool
	streamID uint32
	fc       types.FlowControl
	types.StreamState
}

func (s *streamImpl) ID() uint32            { return s.streamID }
func (s *streamImpl) FC() types.FlowControl { return s.fc }
func (s *streamImpl) End() {
	s.StreamState.End()
	s.pool.release(s)
}

type Opt interface {
	apply(*StreamsPool)
}

type WithInitialWindowSize uint32

func (s WithInitialWindowSize) apply(p *StreamsPool) {
	p.initialWindowSize = uint32(s)
}
