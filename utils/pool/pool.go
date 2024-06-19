package pool

import "sync"

type SlicePool[T any] struct {
	mu sync.Mutex
	s  []T
}

func NewSlicePool[T any]() *SlicePool[T] {
	return new(SlicePool[T])
}

func NewSlicePoolSize[T any](size int) *SlicePool[T] {
	return &SlicePool[T]{s: make([]T, 0, size)}
}

func (p *SlicePool[T]) Acquire() (v T, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	l := len(p.s)
	if l == 0 {
		return v, false
	}

	v = p.s[l-1]
	p.s = p.s[:l-1]
	return v, true
}

func (p *SlicePool[T]) Release(v T) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.s = append(p.s, v)
}
