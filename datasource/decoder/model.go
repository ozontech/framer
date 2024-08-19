package decoder

type Meta struct {
	Name  string
	Value string
}

type Data struct {
	Tag      string
	Method   string // "/" {service name} "/" {method name}
	Metadata []Meta
	Message  []byte
}

func (d *Data) Reset() {
	d.Tag = ""
	d.Method = ""
	d.Metadata = d.Metadata[:0]
	d.Message = d.Message[:0]
}
