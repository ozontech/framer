package multi

import (
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"
)

type Multi struct {
	nested []types.Reporter
	pool   *pool.SlicePool[multiState]
}

func NewMutli(nested ...types.Reporter) *Multi {
	return &Multi{
		nested,
		pool.NewSlicePoolSize[multiState](128),
	}
}

func (m *Multi) Run() error {
	g := new(errgroup.Group)
	for i := range m.nested {
		r := m.nested[i]
		g.Go(r.Run)
	}
	return g.Wait()
}

func (m *Multi) Close() error {
	g := new(errgroup.Group)
	for i := range m.nested {
		r := m.nested[i]
		g.Go(r.Close)
	}
	return g.Wait()
}

func (m *Multi) Acquire(tag string) types.StreamState {
	ms, ok := m.pool.Acquire()
	if !ok {
		ms = make(multiState, len(m.nested))
	}

	for i, r := range m.nested {
		ms[i] = r.Acquire(tag)
	}
	return ms
}

type multiState []types.StreamState

func (s multiState) SetSize(n int) {
	for _, s := range s {
		s.SetSize(n)
	}
}

func (s multiState) OnHeader(name, value string) {
	for _, s := range s {
		s.OnHeader(name, value)
	}
}

func (s multiState) RSTStream(code http2.ErrCode) {
	for _, s := range s {
		s.RSTStream(code)
	}
}

func (s multiState) IoError(err error) {
	for _, s := range s {
		s.IoError(err)
	}
}

func (s multiState) GoAway(errCode http2.ErrCode) {
	for _, s := range s {
		s.GoAway(errCode)
	}
}

func (s multiState) End() {
	for _, s := range s {
		s.End()
	}
}
