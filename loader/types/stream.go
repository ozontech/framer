package types

import (
	"golang.org/x/net/http2"
)

type Streams struct {
	Pool    StreamsPool
	Limiter StreamsLimiter
	Store   StreamStore
}

type StreamsPool interface {
	Acquire(streamID uint32, tag string) Stream
}

type StreamsLimiter interface {
	WaitAllow() // дождаться разрешение лимитера на создание нового стрима
	Release()   // сообщить о завершении стрима
}

type StreamStore interface {
	Set(uint32, Stream)         // добавить стрим в хранилище
	Get(uint32) Stream          // получить стрим из хранилища
	GetAndDelete(uint32) Stream // удалить и вернуть
	Delete(uint32)              // удалить стрим из хранилища
	Each(func(Stream))          // итерируется по всем стримам хранилища
}

type LoaderReporter interface {
	Acquire(tag string, streamID uint32) StreamState
}

type Reporter interface {
	LoaderReporter
	Run() error
	Close() error
}

type StreamState interface {
	RequestError(error)                          // не смогли подготовить запрос: не смогли декодировать запрос, привысили лимиты сервера (SettingMaxHeaderListSize) etc
	FirstByteSent()                              // отправили первый байт
	LastByteSent()                               // отправили последний байт
	SetSize(int)                                 // сообщаем какой размер у данного унарного стрима
	OnHeader(name, value string)                 // сообщаем хедеры по мере их получения
	IoError(error)                               // сообщаем стриму, что получили ошибку ввода/вывода
	RSTStream(code http2.ErrCode)                // если получили RST_STREAM
	GoAway(code http2.ErrCode, debugData []byte) // получен фрейм goaway со stream id > текущего
	Timeout()                                    // случился таймаут стрима
	End()                                        // завершение стрима. отправляет результат в отчет
}

type Stream interface {
	ID() uint32
	FC() FlowControl
	StreamState
}
