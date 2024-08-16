package supersimple

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
	"golang.org/x/net/http2"
)

type Reporter struct {
	pool    *pool.SlicePool[*streamState]
	closeCh chan struct{}

	start time.Time
	ok    atomic.Uint32
	nook  atomic.Uint32
	req   atomic.Uint32
	size  atomic.Uint64

	lastOk   uint32
	lastNook uint32
	lastReq  uint32
	lastSize uint64
	lastTime time.Time
}

func New() *Reporter {
	now := time.Now()
	return &Reporter{
		pool:     pool.NewSlicePoolSize[*streamState](100),
		closeCh:  make(chan struct{}),
		start:    now,
		lastTime: now,
	}
}

func (a *Reporter) Run() error {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	defer a.total()
	for {
		select {
		case now := <-t.C:
			a.report(now)
		case <-a.closeCh:
			return nil
		}
	}
}

func (a *Reporter) Close() error {
	close(a.closeCh)
	return nil
}

func (a *Reporter) Acquire(tag string, streamID uint32) types.StreamState {
	a.req.Add(1)
	ss, ok := a.pool.Acquire()
	if !ok {
		ss = &streamState{reporter: a}
	}
	ss.reset(tag)
	return ss
}

func (a *Reporter) accept(s *streamState) {
	if s.result() {
		a.ok.Add(1)
	} else {
		a.nook.Add(1)
	}

	a.pool.Release(s)
}

func (a *Reporter) addSize(size int) {
	a.size.Add(uint64(size))
}

// var p = message.NewPrinter(language.English)

func (a *Reporter) write(ok, nook, req uint32, size uint64, d time.Duration) {
	total := ok + nook
	miliSeconds := d.Milliseconds()
	if miliSeconds > 0 {
		fmt.Printf(
			"total=%d ok=%d nook=%d req=%d size=%s req/s=%.2f resp/s=%.2f\n",
			total, ok, nook, req,
			humanize.Bytes(size*1000/uint64(miliSeconds)),
			float64(req)*1000/float64(miliSeconds), float64(total)*1000/float64(miliSeconds),
		)
	} else {
		fmt.Printf("total=%d ok=%d nook=%d req=%d\n", total, ok, nook, req)
	}
}

func (a *Reporter) total() {
	fmt.Println("total")
	a.write(a.ok.Load(), a.nook.Load(), a.req.Load(), a.size.Load(), time.Since(a.start))
}

func (a *Reporter) report(now time.Time) {
	ok, nook, req, size, period := a.ok.Load(), a.nook.Load(), a.req.Load(), a.size.Load(), now.Sub(a.lastTime)
	a.write(ok-a.lastOk, nook-a.lastNook, req-a.lastReq, size-a.lastSize, period)
	a.lastOk, a.lastNook, a.lastTime, a.lastReq, a.lastSize = ok, nook, now, req, size
}

type streamState struct {
	reporter *Reporter
	noOk     bool
}

func (s *streamState) reset(_ string) {
	s.noOk = false
}

func (s *streamState) FirstByteSent() {}
func (s *streamState) LastByteSent()  {}

func (s *streamState) SetSize(size int) {
	s.reporter.addSize(size)
}

func (s *streamState) OnHeader(name, value string) {
	if name == "grpc-status" && value != "0" {
		s.noOk = true
	}
}

func (s *streamState) RequestError(error)           { s.noOk = true }
func (s *streamState) IoError(error)                { s.noOk = true }
func (s *streamState) RSTStream(http2.ErrCode)      { s.noOk = true }
func (s *streamState) GoAway(http2.ErrCode, []byte) { s.noOk = true }
func (s *streamState) Timeout()                     { s.noOk = true }

func (s *streamState) result() (ok bool) {
	return !s.noOk
}

func (s *streamState) End() {
	s.reporter.accept(s)
}
