package datasource

import (
	"strings"

	"github.com/ozontech/framer/loader/types"
)

type MetaMiddleware interface {
	IsAllowed(key string) bool
	WriteAdditional(types.HPackFieldWriter)
}

type noopMiddleware struct{}

func (noopMiddleware) IsAllowed(string) bool                  { return true }
func (noopMiddleware) WriteAdditional(types.HPackFieldWriter) {}

type defaultMiddleware struct {
	staticPseudo  []string
	staticRegular []string
	next          MetaMiddleware
}

func newDefaultMiddleware(next MetaMiddleware, additionalHeaders ...string) *defaultMiddleware {
	m := &defaultMiddleware{
		staticPseudo: []string{
			":method", "POST",
			":scheme", "http",
		},
		staticRegular: []string{
			"content-type", "application/grpc",
			"te", "trailers",
		},
		next: noopMiddleware{},
	}

	for i := 0; i < len(additionalHeaders); i += 2 {
		k, v := additionalHeaders[i], additionalHeaders[i+1]
		if strings.HasPrefix(k, ":") {
			m.staticPseudo = append(m.staticPseudo, k, v)
		} else {
			m.staticRegular = append(m.staticRegular, k, v)
		}
	}

	return m
}

func (m *defaultMiddleware) IsAllowed(k string) (allowed bool) {
	if k == "" {
		return false
	}
	// отфильтровываем псевдохедеры из меты т.к.
	// стандартный клиент также псевдохедеры не пропускает
	// и псевдохедерами можно легко сломать стрельбу
	if k[0] == ':' {
		return false
	}
	switch k {
	case "content-type", "te", "grpc-timeout":
		return false
	}
	return m.next.IsAllowed(k)
}

func (m *defaultMiddleware) WriteAdditional(hpack types.HPackFieldWriter) {
	// добавляем статичные псевдохедеры
	staticPseudo := m.staticPseudo
	for i := 0; i < len(staticPseudo); i += 2 {
		//nolint:errcheck // пишем в буфер, это безопасно
		hpack.WriteField(staticPseudo[i], staticPseudo[i+1])
	}

	m.next.WriteAdditional(hpack)

	// добавляем статичные хедеры
	staticRegular := m.staticRegular
	for i := 0; i < len(staticRegular); i += 2 {
		//nolint:errcheck // пишем в буфер, это безопасно
		hpack.WriteField(staticRegular[i], staticRegular[i+1])
	}
}
