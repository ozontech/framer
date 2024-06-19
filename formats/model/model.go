package model

type Meta struct {
	Name  []byte
	Value []byte
}

type Data struct {
	Tag      []byte
	Method   []byte // "/" {service name} "/" {method name}
	Metadata []Meta
	Message  []byte
}

func (d *Data) Reset() {
	d.Tag = d.Tag[:0]
	d.Method = d.Method[:0]
	d.Metadata = d.Metadata[:0]
	d.Message = d.Message[:0]
}

type Marshaler interface {
	MarshalAppend([]byte, *Data) ([]byte, error)
}

type Unmarshaler interface {
	Unmarshal(d *Data, b []byte) error
}

type RequestReader interface {
	ReadNext([]byte) ([]byte, error)
}

type PooledRequestReder interface {
	ReadNext() ([]byte, error)
	Release([]byte)
}

type RequestWriter interface {
	WriteNext([]byte) error
}

type InputFormat struct {
	Reader  PooledRequestReder
	Decoder Unmarshaler
}

type OutputFormat struct {
	Writer  RequestWriter
	Encoder Marshaler
}
