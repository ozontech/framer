package pool

import (
	"math"
	"sync"

	"github.com/ozontech/framer/loader/flowcontrol"
	"github.com/ozontech/framer/loader/types"
)

const defaultInitialWindowSize = 65535

type StreamsPool struct {
	reporter types.LoaderReporter

	cond *sync.Cond
	pool []*streamImpl

	inUse                uint32
	maxConcurrentStreams uint32
	initialWindowSize    uint32
}

func NewStreamsPool(
	reporter types.LoaderReporter,
	initSize uint32,
	limit uint32, // limit = 0 интерпретируется как неограниченное количество
) *StreamsPool {
	if limit == 0 {
		limit = math.MaxUint32
	}
	return &StreamsPool{
		reporter:             reporter,
		cond:                 sync.NewCond(&sync.Mutex{}),
		pool:                 make([]*streamImpl, 0, initSize),
		maxConcurrentStreams: limit,
		initialWindowSize:    defaultInitialWindowSize,
	}
}

func (p *StreamsPool) Acquire(streamID uint32, tag string) types.Stream {
	var stream *streamImpl
	p.cond.L.Lock()
	if p.inUse >= p.maxConcurrentStreams {
		p.cond.Wait()
	}

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
	stream.StreamState = p.reporter.Acquire(tag)

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

func (p *StreamsPool) SetInitialWindowSize(size uint32) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	p.initialWindowSize = size
}

func (p *StreamsPool) SetLimit(limit uint32) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()
	p.maxConcurrentStreams = limit
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
