package limiter

import (
	"sync"

	"github.com/ozontech/framer/loader/types"
)

func New(quota uint32) types.StreamsLimiter {
	if quota == 0 {
		return noopLimiter{}
	}
	return newLimiter(quota)
}

type noopLimiter struct{}

func (noopLimiter) WaitAllow() {}
func (noopLimiter) Release()   {}

type limiter struct {
	quota uint32
	cond  *sync.Cond
}

func newLimiter(quota uint32) *limiter {
	return &limiter{quota, sync.NewCond(&sync.Mutex{})}
}

func (l *limiter) WaitAllow() {
	l.cond.L.Lock()
	defer l.cond.L.Unlock()
	for l.quota == 0 {
		l.cond.Wait()
	}

	l.quota--
}

func (l *limiter) Release() {
	l.cond.L.Lock()
	defer l.cond.Signal()
	defer l.cond.L.Unlock()

	l.quota++
}
