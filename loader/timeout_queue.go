package loader

import (
	"sync"
	"time"
)

var NewTimeoutQueue = NewTimeoutSliceQueue

// очередь проверки таймаута на основе слайса.
type timeoutQueueItem struct {
	streamID uint32
	deadline time.Time
}

type timeoutSliceQueue struct {
	timeout time.Duration
	queue   []timeoutQueueItem
	cond    *sync.Cond

	done chan struct{}
}

func NewTimeoutSliceQueue(timeout time.Duration) TimeoutQueue {
	return &timeoutSliceQueue{
		timeout: timeout,
		queue:   make([]timeoutQueueItem, 0, 10),
		cond:    sync.NewCond(&sync.Mutex{}),
		done:    make(chan struct{}),
	}
}

func (q *timeoutSliceQueue) Add(streamID uint32) {
	q.cond.L.Lock()
	q.queue = append(q.queue, timeoutQueueItem{
		streamID,
		time.Now().Add(q.timeout),
	})
	q.cond.L.Unlock()

	q.cond.Signal()
}

func (q *timeoutSliceQueue) Next() (uint32, bool) {
	q.cond.L.Lock()
	for len(q.queue) == 0 {
		select {
		case <-q.done:
			return 0, false
		default:
			q.cond.Wait()
		}
	}
	nextItem := q.queue[0]
	q.queue = q.queue[1:]
	q.cond.L.Unlock()

	select {
	case <-q.done:
		return 0, false
	case <-time.After(time.Until(nextItem.deadline)):
		return nextItem.streamID, true
	}
}

func (q *timeoutSliceQueue) Close() {
	close(q.done)
	q.cond.Signal()
}
