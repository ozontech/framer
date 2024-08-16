package lru

import (
	"container/list"
	"sync"
)

type LRU struct {
	maxSize int
	items   map[string]*list.Element
	list    *list.List
	mu      sync.Mutex
}

func New(maxSize int) *LRU {
	if maxSize < 1 {
		panic("assertion error: maxSize < 1")
	}
	return &LRU{
		maxSize: maxSize,
		items:   make(map[string]*list.Element, maxSize),
		list:    list.New(),
	}
}

// GetOrAdd fetch item from lru and increase eviction order or create
func (l *LRU) GetOrAdd(keyB []byte) string {
	l.mu.Lock()
	defer l.mu.Unlock()

	element, ok := l.items[string(keyB)]
	if ok {
		l.list.MoveToFront(element)
		return element.Value.(string)
	}

	if len(l.items) >= l.maxSize {
		element = l.list.Back()
		l.list.Remove(element)
		delete(l.items, element.Value.(string))
	}

	keyS := string(keyB)
	element = l.list.PushFront(keyS)
	l.items[keyS] = element
	return keyS
}
