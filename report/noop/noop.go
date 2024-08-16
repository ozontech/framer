package noop

import (
	"github.com/ozontech/framer/loader/types"
	"golang.org/x/net/http2"
)

type Noop struct {
	close chan struct{}
}

func New() *Noop {
	return &Noop{make(chan struct{})}
}

func (m *Noop) Run() error {
	<-m.close
	return nil
}

func (m *Noop) Close() error {
	close(m.close)
	return nil
}

func (m *Noop) Acquire(string, uint32) types.StreamState {
	return &noopState{}
}

type noopState struct{}

func (s *noopState) FirstByteSent()               {}
func (s *noopState) LastByteSent()                {}
func (s *noopState) RequestError(error)           {}
func (s *noopState) SetSize(int)                  {}
func (s *noopState) OnHeader(string, string)      {}
func (s *noopState) RSTStream(http2.ErrCode)      {}
func (s *noopState) IoError(error)                {}
func (s *noopState) GoAway(http2.ErrCode, []byte) {}
func (s *noopState) Timeout()                     {}
func (s *noopState) End()                         {}
