package types

import (
	"golang.org/x/net/http2"
)

type StreamStore interface {
	Set(uint32, Stream)         // добавить стрим в хранилище
	Get(uint32) Stream          // получить стрим из хранилища
	GetAndDelete(uint32) Stream // удалить и вернуть
	Delete(uint32)              // удалить стрим из хранилища
	Each(func(Stream))          // итерируется по всем стримам хранилища
}

type LoaderReporter interface {
	Acquire(tag string) StreamState
}

type Reporter interface {
	LoaderReporter
	Run() error
	Close() error
}

type StreamState interface {
	SetSize(int)                  // сообщаем какой размер у данного унарного стрима
	OnHeader(name, value string)  // сообщаем хедеры по мере их получения
	IoError(err error)            // сообщаем стриму, что получили ошибку ввода/вывода
	RSTStream(code http2.ErrCode) // если получили RST_STREAM
	GoAway(code http2.ErrCode)    // получен фрейм goaway со stream id > текущего
	End()                         // завершение стрима. отправляет результат в отчет
}

type Stream interface {
	ID() uint32
	FC() FlowControl
	StreamState
}
