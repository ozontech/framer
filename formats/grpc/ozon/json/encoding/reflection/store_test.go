//nolint:goconst
package reflection

import (
	"bytes"
	"sort"
	"strconv"
	"sync"
	"testing"
)

func BenchmarkMapStore(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		m := make(map[string][]byte)
		var sm sync.Map
		items := []storeItem{}
		for i := 0; i < n; i++ {
			k := "/test.api.TestApi/Test" + strconv.Itoa(i)
			v := []byte(k)

			m[k] = v
			sm.Store(k, v)
			items = append(items, storeItem{
				k: []byte(k),
				v: v,
			})
		}
		s := newDynamicMessagesStore(items)

		b.Run("map"+strconv.Itoa(n), func(b *testing.B) {
			k := []byte("/test.api.TestApi/Test" + strconv.Itoa(b.N%n))
			for i := 0; i < b.N; i++ {
				_ = m[string(k)]
			}
		})

		b.Run("binSearchStore"+strconv.Itoa(n), func(b *testing.B) {
			k := []byte("/test.api.TestApi/Test" + strconv.Itoa(b.N%n))
			for i := 0; i < b.N; i++ {
				s.Get(k)
			}
		})

		b.Run("syncmap"+strconv.Itoa(n), func(b *testing.B) {
			k := []byte("/test.api.TestApi/Test" + strconv.Itoa(b.N%n))
			for i := 0; i < b.N; i++ {
				v, ok := sm.Load(string(k))
				if ok {
					_ = v.([]byte)
				}
			}
		})
	}
}

type storeItem struct {
	k []byte
	v []byte
}

type binSearchStore struct {
	items []storeItem
}

func (s *binSearchStore) Len() int      { return len(s.items) }
func (s *binSearchStore) Swap(i, j int) { s.items[i], s.items[j] = s.items[j], s.items[i] }
func (s *binSearchStore) Less(i, j int) bool {
	return bytes.Compare(s.items[i].k, s.items[j].k) < 0
}

func (s *binSearchStore) Get(k []byte) []byte {
	i := sort.Search(len(s.items), func(i int) bool {
		return bytes.Compare(s.items[i].k, k) >= 0
	})
	if i == len(s.items) {
		return nil
	}
	item := s.items[i]
	if bytes.Equal(item.k, k) {
		return item.v
	}

	return nil
}

func newDynamicMessagesStore(items []storeItem) *binSearchStore {
	store := &binSearchStore{items}
	sort.Sort(store)
	return store
}
