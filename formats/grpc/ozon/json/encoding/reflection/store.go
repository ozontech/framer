package reflection

import (
	"bytes"
	"sync"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
)

type DynamicMessagesStore interface {
	// must return nil if not found
	Get(methodName []byte) (message *dynamic.Message, release func())
}

type dynamicMessagesStore struct {
	items map[string]*sync.Pool
}

func (s *dynamicMessagesStore) Get(methodName []byte) (*dynamic.Message, func()) {
	pool, ok := s.items[string(methodName)]
	if !ok {
		return nil, nil
	}
	message := pool.Get().(*dynamic.Message)
	return message, func() { pool.Put(message) }
}

func NewDynamicMessagesStore(descriptors []*desc.MethodDescriptor) DynamicMessagesStore {
	items := make(map[string]*sync.Pool, len(descriptors))
	for _, descriptor := range descriptors {
		descriptor := descriptor

		fqn := string(NormalizeMethod([]byte(descriptor.GetFullyQualifiedName())))
		items[fqn] = &sync.Pool{New: func() interface{} {
			return dynamic.NewMessage(descriptor.GetInputType())
		}}
	}
	return &dynamicMessagesStore{items}
}

// NormalizeMethod - приводит метод к стандартному виду '/package.Service/Call'
// из 'package.Service.Call'.
func NormalizeMethod(method []byte) []byte {
	if len(method) == 0 || method[0] == '/' {
		return method
	}

	ind := bytes.LastIndexByte(method, '.')
	if ind != -1 {
		method[ind] = '/'
	}
	method = append(method, 0x0)
	copy(method[1:], method)
	method[0] = '/'

	return method
}
