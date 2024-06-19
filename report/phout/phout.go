package phout

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"syscall"
	"time"

	"github.com/ozontech/framer/loader/types"
	"github.com/ozontech/framer/utils/pool"
	"golang.org/x/net/http2"
)

var now = time.Now

type Reporter struct {
	closeCh chan struct{}
	w       *bufio.Writer
	ch      chan *streamState
	pool    *pool.SlicePool[*streamState]
	timeout time.Duration
}

func New(w io.Writer, timeout time.Duration) *Reporter {
	return &Reporter{
		make(chan struct{}),
		bufio.NewWriter(w),
		make(chan *streamState, 256),
		pool.NewSlicePoolSize[*streamState](256),
		timeout,
	}
}

func (r *Reporter) Run() error {
	for s := range r.ch {
		_, err := r.w.Write(s.result())
		if err != nil {
			return fmt.Errorf("write: %w", err)
		}
		r.pool.Release(s)
	}
	return r.w.Flush()
}

func (r *Reporter) Close() error {
	close(r.ch)
	return nil
}

func (r *Reporter) Acquire(tag string) types.StreamState {
	ss, ok := r.pool.Acquire()
	if !ok {
		ss = &streamState{
			reportLine: make([]byte, 128),
			reporter:   r,
			timeout:    r.timeout,
		}
	}
	ss.reset(tag)
	return ss
}

func (r *Reporter) accept(s *streamState) {
	r.ch <- s
}

type streamState struct {
	reportLine []byte

	reporter *Reporter
	timeout  time.Duration

	grpcCodeHeader  string
	http2CodeHeader string

	ioErr         error
	rstStreamCode *http2.ErrCode
	goAwayCode    *http2.ErrCode

	reqSize   int
	startTime time.Time
	endTime   time.Time
	tag       string
}

func (s *streamState) reset(tag string) {
	s.tag = tag
	s.startTime = now()

	s.grpcCodeHeader = ""
	s.http2CodeHeader = ""

	s.ioErr = nil
	s.goAwayCode = nil
	s.rstStreamCode = nil
	s.reqSize = 0
}

func (s *streamState) SetSize(size int) {
	s.reqSize = size
}

func (s *streamState) OnHeader(name, value string) {
	switch name {
	case "grpc-status":
		s.grpcCodeHeader = value
	case ":status":
		s.http2CodeHeader = value
	}
}

func (s *streamState) IoError(err error) {
	s.ioErr = err
}

func (s *streamState) RSTStream(code http2.ErrCode) {
	s.rstStreamCode = &code
}

func (s *streamState) GoAway(code http2.ErrCode) {
	s.goAwayCode = &code
}

const tabChar = '\t'

func (s *streamState) result() []byte {
	s.reportLine = s.reportLine[:0]
	s.reportLine = strconv.AppendInt(s.reportLine, s.startTime.Unix(), 10)
	s.reportLine = append(s.reportLine, '.')
	s.reportLine = strconv.AppendInt(s.reportLine, int64(s.startTime.Nanosecond()/1e6), 10)
	s.reportLine = append(s.reportLine, tabChar)
	s.reportLine = append(s.reportLine, []byte(s.tag)...)
	s.reportLine = append(s.reportLine, tabChar)

	// keyRTTMicro     = iota
	rtt := s.endTime.Sub(s.startTime).Microseconds()

	s.reportLine = strconv.AppendInt(s.reportLine, rtt, 10)
	s.reportLine = append(s.reportLine, tabChar)

	// keyConnectMicro // TODO (skipor): set all for HTTP using httptrace and helper structs
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keySendMicro
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keyLatencyMicro
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keyReceiveMicro
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keyIntervalEventMicro // TODO: understand WTF is that mean and set it right.
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keyRequestBytes
	s.reportLine = strconv.AppendInt(s.reportLine, int64(s.reqSize), 10)
	s.reportLine = append(s.reportLine, tabChar)
	// keyResponseBytes
	s.reportLine = append(s.reportLine, '0', tabChar)
	// keyErrno
	var errNo syscall.Errno
	if s.ioErr != nil {
		if !errors.As(s.ioErr, &errNo) {
			errNo = 999
		}
		s.reportLine = strconv.AppendInt(s.reportLine, int64(errNo), 10)
		s.reportLine = append(s.reportLine, tabChar)
	} else {
		s.reportLine = append(s.reportLine, '0', tabChar)
	}
	// keyProtoCode
	switch {
	case s.rstStreamCode != nil:
		s.reportLine = append(s.reportLine, "rst_"...)
		s.reportLine = strconv.AppendInt(s.reportLine, int64(*s.rstStreamCode), 10)
	case s.goAwayCode != nil:
		s.reportLine = append(s.reportLine, "goaway_"...)
		s.reportLine = strconv.AppendInt(s.reportLine, int64(*s.goAwayCode), 10)
	case s.endTime.Sub(s.startTime) > s.timeout:
		s.reportLine = append(s.reportLine, "grpc_4"...)
	case s.http2CodeHeader == "":
		s.reportLine = append(s.reportLine, "http2_1"...) // protocol errro
	case s.grpcCodeHeader != "":
		s.reportLine = append(s.reportLine, "grpc_"...)
		s.reportLine = append(s.reportLine, []byte(s.grpcCodeHeader)...)
	default:
		s.reportLine = append(s.reportLine, "http2_1"...) // protocol error
	}
	s.reportLine = append(s.reportLine, '\n')
	return s.reportLine
}

func (s *streamState) End() {
	s.endTime = now()
	s.reporter.accept(s)
}
