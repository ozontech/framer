package sender

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/ozontech/framer/consts"
	streamsPool "github.com/ozontech/framer/loader/streams/pool"
	"github.com/ozontech/framer/loader/types"
	hpackwrapper "github.com/ozontech/framer/utils/hpack_wrapper"
)

type writeCmd struct {
	bufs      net.Buffers
	onRelease func()
}

type frame struct {
	chunks [3][]byte
	types.Releaser
}

type Sender struct {
	streamPool  *streamsPool.StreamsPool
	streamStore types.StreamStore

	fcConn          types.FlowControl
	conn            io.Writer
	hpackEncWrapper *hpackwrapper.Wrapper
	streamID        uint32

	writeCmdChan chan writeCmd

	priorityFrameChan chan []byte
	frameChan         chan frame
}

func NewSender(
	conn io.Writer,
	fcConn types.FlowControl,
	priorityChunkChan chan []byte,
	streamPool *streamsPool.StreamsPool,
	streamStore types.StreamStore,
) *Sender {
	return &Sender{
		streamPool,
		streamStore,
		fcConn,
		conn,
		hpackwrapper.NewWrapper(),
		1,

		make(chan writeCmd),
		priorityChunkChan,
		make(chan frame, 1000),
	}
}

func (s *Sender) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go s.processLoop(ctx)
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

func (s *Sender) processLoop(ctx context.Context) {
	var (
		writeCmdChan      chan<- writeCmd = s.writeCmdChan
		priorityChunkChan <-chan []byte   = s.priorityFrameChan
		frameChan         <-chan frame    = s.frameChan
	)

	rotator := newRotator()
	buffers, releasers := rotator.Rotate()

	lastWrite := time.Now()
	doWrite := func(b net.Buffers) {
		if len(b) == 0 {
			return
		}

		writeCmdChan <- writeCmd{b, releasers.Release}
		buffers, releasers = rotator.Rotate()
		lastWrite = time.Now()
	}

	timer := time.NewTimer(consts.SendBatchTimeout)
l:
	for {
		timer.Reset(consts.SendBatchTimeout - time.Since(lastWrite))
		select {
		case b := <-priorityChunkChan:
			// если получили приритетный фрейм, то он должен записаться первым
			buffers[0] = b
			doWrite(buffers)
		default:
			select {
			case b := <-priorityChunkChan:
				buffers[0] = b
				doWrite(buffers)
			case f, ok := <-frameChan:
				if !ok {
					doWrite(buffers[1:])
					break l
				}
				if len(f.chunks)+len(buffers) > cap(buffers) {
					doWrite(buffers[1:])
				}

				for _, c := range f.chunks {
					if c == nil {
						break
					}

					buffers = append(buffers, c)
				}
				if f.Releaser != nil {
					releasers = append(releasers, f.Releaser)
				}
			case <-ctx.Done():
				doWrite(buffers[1:])
				return
			// time.After аллоцирует память в куче т.к. на каждый вызов создает канал
			// => сильно медленее переиспользования таймера
			case <-timer.C:
				doWrite(buffers[1:])
			}
		}
	}

	for {
		select {
		case b := <-priorityChunkChan:
			// если получили приритетный фрейм, то он должен записаться первым
			buffers[0] = b
			doWrite(buffers)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Sender) Send(a types.Req) {
	s.send(a)
}

// var n atomic.Int64
//
// func init() {
// 	go func() {
// 		p := message.NewPrinter(language.English)
// 		for range time.Tick(time.Second) {
// 			p.Printf("%d\n", n.Swap(0))
// 		}
// 	}()
// }
//
// const (
// 	acquire      = true
// 	setupRequest = true
// 	saveToStore  = true
// )
//
// func (s *Sender) sendNoop1(a types.Req) {
// 	n.Add(1)
//
// 	s.streamID += 2
// 	if setupRequest {
// 		a.SetUp(s.streamID, s.hpackEncWrapper)
// 	}
// 	a.Release()
//
// 	if acquire {
// 		stream := s.streamPool.Acquire(s.streamID, a.Tag())
// 		stream.SetSize(a.Size())
// 		stream.End()
//
// 		if saveToStore {
// 			s.streamStore.Set(s.streamID, stream)
// 			s.streamStore.Delete(s.streamID)
// 		}
// 	}
// }
//
// func (s *Sender) sendNoop2(a types.Req) {
// 	n.Add(1)
// 	s.streamID += 2
// 	// a.SetUp(s.streamID, s.hpackEncWrapper)
//
// 	stream := s.streamPool.Acquire(s.streamID, a.Tag())
// 	a.Release()
// 	stream.End()
// }

func (s *Sender) send(a types.Req) {
	// n.Add(1)
	s.streamID += 2
	frames := a.SetUp(s.streamID, s.hpackEncWrapper)

	stream := s.streamPool.Acquire(s.streamID, a.Tag())
	stream.SetSize(a.Size())
	s.streamStore.Set(s.streamID, stream)

	var (
		i         int
		lastIndex = len(frames) - 1
	)
	for {
		f := frames[i]
		if !stream.FC().Wait(f.FlowControlPrice) ||
			!s.fcConn.Wait(f.FlowControlPrice) {
			return
		}
		if i == lastIndex {
			s.frameChan <- frame{chunks: f.Chunks, Releaser: a}
			break
		}
		s.frameChan <- frame{chunks: f.Chunks}
		i++
	}
}

func (s *Sender) Flush() {
	close(s.frameChan)
}

type rotatorItem struct {
	buffers   [consts.ChunksBufferSize][]byte
	releasers [consts.ChunksBufferSize]types.Releaser
}
type rotator struct {
	// net.Buffers.WriteTo может уменьшать капасити слайса,
	// поэтому, чтобы переиспользовать память используется массив, с которого создается слайс
	current *rotatorItem
	next    *rotatorItem
}

func newRotator() *rotator {
	return &rotator{new(rotatorItem), new(rotatorItem)}
}

func (r *rotator) Rotate() ([][]byte, releasers) {
	r.current, r.next = r.next, r.current
	return r.current.buffers[:1], r.current.releasers[:0]
}

type releasers []types.Releaser

func (rr releasers) Release() {
	for _, r := range rr {
		r.Release()
	}
}
