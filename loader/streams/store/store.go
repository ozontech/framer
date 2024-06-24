package store

import (
	"sync"

	"github.com/ozontech/framer/loader/types"
)

type StreamsNoop struct{}

func NewStreamsNoop() StreamsNoop { return StreamsNoop{} }

func (m StreamsNoop) Each(func(types.Stream))          {}
func (m StreamsNoop) Set(uint32, types.Stream)         {}
func (m StreamsNoop) Get(uint32) types.Stream          { return nil }
func (m StreamsNoop) GetAndDelete(uint32) types.Stream { return nil }
func (m StreamsNoop) Delete(uint32)                    {}

type StreamsMapUnlocked map[uint32]types.Stream

func NewStreamsMapUnlocked(size int) StreamsMapUnlocked {
	return make(map[uint32]types.Stream, size)
}

func (m StreamsMapUnlocked) Each(fn func(types.Stream)) {
	for _, stream := range m {
		fn(stream)
	}
}
func (m StreamsMapUnlocked) Set(id uint32, stream types.Stream) { m[id] = stream }
func (m StreamsMapUnlocked) Get(id uint32) types.Stream         { return m[id] }

func (m StreamsMapUnlocked) GetAndDelete(id uint32) types.Stream {
	stream := m.Get(id)
	if stream != nil {
		m.Delete(id)
	}
	return stream
}
func (m StreamsMapUnlocked) Delete(id uint32) { delete(m, id) }

// StreamsMap имплементация хранилища стримов используя map
type StreamsMap struct {
	m  StreamsMapUnlocked
	mu *sync.RWMutex
}

func NewStreamsMap() *StreamsMap {
	return &StreamsMap{
		m:  NewStreamsMapUnlocked(1024),
		mu: &sync.RWMutex{},
	}
}

func (s *StreamsMap) Each(fn func(types.Stream)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.m.Each(fn)
}

func (s *StreamsMap) Set(id uint32, stream types.Stream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.m.Set(id, stream)
}

func (s *StreamsMap) Get(id uint32) types.Stream {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.m.Get(id)
}

func (s *StreamsMap) GetAndDelete(id uint32) types.Stream {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.m.GetAndDelete(id)
}

func (s *StreamsMap) Delete(id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.m.Delete(id)
}

// ShardedStreamsMap имплементация хранилища стримов используя шардированный map
type ShardedStreamsMap struct {
	shards []types.StreamStore
	max    uint32
}

func NewShardedStreamsMap(size uint32, build func() types.StreamStore) *ShardedStreamsMap {
	shards := make([]types.StreamStore, size*2)
	for i := 1; i < len(shards); i += 2 {
		shards[i] = build()
	}
	return &ShardedStreamsMap{shards, size - 1}
}

func (s *ShardedStreamsMap) shard(id uint32) types.StreamStore {
	return s.shards[id&s.max]
}

func (s *ShardedStreamsMap) Each(fn func(types.Stream)) {
	for i := 1; i < len(s.shards); i += 2 {
		shard := s.shards[i]
		shard.Each(fn)
	}
}

func (s *ShardedStreamsMap) Set(id uint32, stream types.Stream) {
	s.shard(id).Set(id, stream)
}

func (s *ShardedStreamsMap) Get(id uint32) types.Stream {
	return s.shard(id).Get(id)
}

func (s *ShardedStreamsMap) GetAndDelete(id uint32) types.Stream {
	return s.shard(id).GetAndDelete(id)
}

func (s *ShardedStreamsMap) Delete(id uint32) {
	s.shard(id).Delete(id)
}
