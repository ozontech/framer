package sender

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/ozontech/framer/consts"
	"github.com/ozontech/framer/loader/types"
	hpackwrapper "github.com/ozontech/framer/utils/hpack_wrapper"
	"go.uber.org/zap"
)

type writeCmd struct {
	bufs      net.Buffers
	onRelease func()
}

type frame struct {
	chunks [3][]byte
	cbs    [3]func()
}

type Sender struct {
	log *zap.Logger

	maxFrameSize      int
	maxHeaderListSize int
	streams           types.Streams

	fcConn          types.FlowControl
	conn            io.Writer
	hpackEncWrapper *hpackwrapper.Wrapper
	streamID        uint32

	writeCmdChan chan writeCmd

	priorityFrameChan chan []byte
	frameChan         chan frame

	disableSendBatching bool
}

func NewSender(
	log *zap.Logger,
	conn io.Writer,
	fcConn types.FlowControl,
	priorityFrameChan chan []byte,
	streams types.Streams,
	hpackEncWrapper *hpackwrapper.Wrapper,
	maxFrameSize int,
	maxHeaderListSize int,
	disableSendBatching bool,
) *Sender {
	return &Sender{
		log,
		maxFrameSize, maxHeaderListSize,

		streams,
		fcConn,
		conn,
		hpackEncWrapper,
		1,

		make(chan writeCmd),
		priorityFrameChan,
		make(chan frame, 16),
		disableSendBatching,
	}
}

func (s *Sender) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.disableSendBatching {
		go instantProcessor(
			ctx,
			s.writeCmdChan,
			s.priorityFrameChan,
			s.frameChan,
		)
	} else {
		go batchedProcessor(
			ctx,
			s.writeCmdChan,
			s.priorityFrameChan,
			s.frameChan,
		)
	}
	return s.sendLoop(ctx)
}

func (s *Sender) sendLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case cmd := <-s.writeCmdChan:
			_, err := cmd.bufs.WriteTo(s.conn)
			cmd.onRelease()
			if err != nil {
				return err
			}
		}
	}
}

func instantProcessor(
	ctx context.Context,
	writeCmdChan chan<- writeCmd,
	priorityChunkChan <-chan []byte,
	frameChan <-chan frame,
) {
	buffers, cbs := make([][]byte, 0, 3), make(multiCB, 0, 3)
	buffersNext, cbsNext := make([][]byte, 0, 3), make(multiCB, 0, 3)

	doWrite := func() {
		writeCmdChan <- writeCmd{buffers, cbs.Call}
		buffers, cbs, buffersNext, cbsNext = buffersNext[:0], cbsNext[:0], buffers, cbs
	}

l:
	for {
		select {
		case b := <-priorityChunkChan:
			buffers = append(buffers, b)
		default:
			select {
			case b := <-priorityChunkChan:
				buffers = append(buffers, b)
			case f, ok := <-frameChan:
				if !ok {
					break l
				}

				for _, c := range f.chunks {
					if c == nil {
						break
					}
					buffers = append(buffers, c)
				}
				for _, cb := range f.cbs {
					if cb != nil {
						cbs = append(cbs, cb)
					}
				}
			case <-ctx.Done():
				return
			}
		}
		doWrite()
	}

	for {
		select {
		case b := <-priorityChunkChan:
			buffers = append(buffers, b)
		case <-ctx.Done():
			return
		}
		doWrite()
	}
}

func batchedProcessor(
	ctx context.Context,
	writeCmdChan chan<- writeCmd,
	priorityChunkChan <-chan []byte,
	frameChan <-chan frame,
) {
	buffers, cbs := make([][]byte, 1, 16), make(multiCB, 0, 16)
	buffersNext, cbsNext := make([][]byte, 1, 16), make(multiCB, 0, 16)

	realTimer := time.NewTimer(consts.SendBatchTimeout)
	var noopTimerChan <-chan time.Time
	timerChan := noopTimerChan

	lastWrite := time.Now()
	doWrite := func(withFirst bool) {
		var b net.Buffers
		if withFirst {
			b = buffers[:]
		} else {
			b = buffers[1:]
		}
		if len(b) == 0 {
			return
		}

		writeCmdChan <- writeCmd{b, cbs.Call}
		buffers, cbs, buffersNext, cbsNext = buffersNext[:1], cbsNext[:0], buffers, cbs
		lastWrite = time.Now()
		timerChan = noopTimerChan
	}

l:
	for {
		select {
		case b := <-priorityChunkChan:
			// если получили приритетный фрейм, то он должен записаться первым
			buffers[0] = b
			doWrite(true)
		default:
			select {
			case b := <-priorityChunkChan:
				buffers[0] = b
				doWrite(true) // тут обязательно реслайситься т.к. net.Buffers изменяет слайс
			case f, ok := <-frameChan:
				timerChan = realTimer.C

				if !ok {
					doWrite(false)
					break l
				}
				if len(f.chunks)+len(buffers) > 2048 {
					doWrite(false)
				}

				for _, c := range f.chunks {
					if c == nil {
						break
					}

					buffers = append(buffers, c)
				}
				for _, cb := range f.cbs {
					if cb != nil {
						cbs = append(cbs, cb)
					}
				}
			case <-ctx.Done():
				doWrite(false)
				return
			// time.After аллоцирует память в куче т.к. на каждый вызов создает канал
			// => сильно медленее переиспользования таймера
			case <-timerChan:
				doWrite(false)
			}
		}
		realTimer.Reset(consts.SendBatchTimeout - time.Since(lastWrite))
	}

	for {
		select {
		case b := <-priorityChunkChan:
			// если получили приритетный фрейм, то он должен записаться первым
			buffers[0] = b
			doWrite(true)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Sender) Send(a types.Req) {
	s.send(a)
}

func (s *Sender) send(a types.Req) {
	// n.Add(1)
	s.streamID += 2

	s.streams.Limiter.WaitAllow()
	stream := s.streams.Pool.Acquire(s.streamID, a.Tag())

	frames, err := a.SetUp(
		s.maxFrameSize, s.maxHeaderListSize,
		s.streamID, s.hpackEncWrapper,
	)
	if err != nil {
		stream.RequestError(err)
		stream.End()
		return
	}
	stream.SetSize(a.Size())

	s.streams.Store.Set(s.streamID, stream)

	lastIndex := len(frames) - 1
	for i := 0; i <= lastIndex; i++ {
		f := frames[i]
		if !stream.FC().Wait(f.FlowControlPrice) ||
			!s.fcConn.Wait(f.FlowControlPrice) {
			return
		}
		//nolint:govet
		frame := frame{chunks: f.Chunks}
		if i == 0 {
			frame.cbs[0] = stream.FirstByteSent
		}
		if i == lastIndex {
			frame.cbs[1] = stream.LastByteSent
			frame.cbs[2] = a.Release
		}
		s.frameChan <- frame
	}
}

func (s *Sender) Flush() {
	close(s.frameChan)
}

type multiCB []func()

func (m multiCB) Call() {
	for _, cb := range m {
		cb()
	}
}
