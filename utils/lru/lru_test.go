package lru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLRU(t *testing.T) {
	t.Parallel()

	a := assert.New(t)
	l := New(3)
	l.GetOrAdd([]byte("one"))
	l.GetOrAdd([]byte("two"))
	l.GetOrAdd([]byte("three"))
	l.GetOrAdd([]byte("one"))
	a.Len(l.items, 3)
	a.Equal(l.list.Len(), 3)
	l.GetOrAdd([]byte("four"))
	a.Len(l.items, 3)
	a.Equal(l.list.Len(), 3)

	lruOrder := []string{"four", "one", "three"}
	a.Len(l.items, len(lruOrder))
	el := l.list.Front()
	for _, v := range lruOrder {
		_, ok := l.items[v]
		a.True(ok)
		a.Equal(el.Value, v)
		el = el.Next()
	}
}
