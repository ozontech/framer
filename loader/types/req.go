package types

import "io"

type HPackFieldWriter interface {
	SetWriter(w io.Writer)
	WriteField(k, v string)
}

type Req interface {
	SetUp(
		maxFramePayloadLen int,
		maxHeaderListSize int,
		streamID uint32,
		fieldWriter HPackFieldWriter,
	) ([]Frame, error)
	FullMethodName() string
	Tag() string
	Size() int
	Releaser
}

type Releaser interface {
	Release()
}

type Frame struct {
	Chunks           [3][]byte
	FlowControlPrice uint32
}

type DataSource interface {
	Fetch() (Req, error)
}
