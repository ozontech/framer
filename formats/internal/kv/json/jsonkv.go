package jsonkv

import (
	"bytes"
	"sort"

	"github.com/mailru/easyjson/jlexer"
	"github.com/ozontech/framer/formats/internal/json"
	"github.com/ozontech/framer/formats/model"
)

type metaSorter []model.Meta

func (m metaSorter) Len() int           { return len(m) }
func (m metaSorter) Less(i, j int) bool { return bytes.Compare(m[i].Name, m[j].Name) < 0 }
func (m metaSorter) Swap(i, j int) {
	m[i], m[j] = m[j], m[i]
}

type MultiVal struct{}

func NewMultiVal() MultiVal {
	return MultiVal{}
}

func (MultiVal) MarshalAppend(b []byte, meta []model.Meta) []byte {
	if len(meta) == 0 {
		return append(b, "{}"...)
	}

	sort.Sort(metaSorter(meta))

	b = append(b, '{')
	if len(meta) > 0 {
		m := meta[0]
		b = json.EscapeStringAppend(b, m.Name)
		b = append(b, ":["...)
		b = json.EscapeStringAppend(b, m.Value)
	}
	for i := 1; i < len(meta); i++ {
		m := meta[i]
		isNew := !bytes.Equal(m.Name, meta[i-1].Name)
		if !isNew {
			b = append(b, ',')
			b = json.EscapeStringAppend(b, m.Value)
			continue
		}

		b = append(b, "],"...)
		b = json.EscapeStringAppend(b, m.Name)
		b = append(b, ":["...)
		b = json.EscapeStringAppend(b, m.Value)
	}
	return append(b, "]}"...)
}

func (MultiVal) UnmarshalAppend(buf []model.Meta, bytes []byte) ([]model.Meta, error) {
	in := jlexer.Lexer{Data: bytes}

	in.Delim('{')
	for !in.IsDelim('}') {
		key := in.UnsafeBytes()

		in.WantColon()

		in.Delim('[')
		for !in.IsDelim(']') {
			buf = append(buf, model.Meta{Name: key, Value: in.UnsafeBytes()})
			in.WantComma()
		}
		in.Delim(']')

		in.WantComma()
	}
	in.Delim('}')
	in.Consumed()

	return buf, in.Error()
}

type SingleVal struct {
	opts *options
}

func NewSingleVal(optFns ...Option) SingleVal {
	return SingleVal{applyOptions(optFns...)}
}

func (SingleVal) MarshalAppend(b []byte, meta []model.Meta) []byte {
	b = append(b, '{')
	i := 0
	for ; i < len(meta)-1; i++ {
		k, v := meta[i].Name, meta[i].Value
		b = json.EscapeStringAppend(b, k)
		b = append(b, ':')
		b = json.EscapeStringAppend(b, v)
		b = append(b, ',')
	}
	for ; i < len(meta); i += 2 {
		k, v := meta[i].Name, meta[i].Value
		b = json.EscapeStringAppend(b, k)
		b = append(b, ':')
		b = json.EscapeStringAppend(b, v)
	}
	return append(b, '}')
}

func (SingleVal) UnmarshalAppend(buf []model.Meta, bytes []byte) ([]model.Meta, error) {
	in := jlexer.Lexer{Data: bytes}

	in.Delim('{')
	for !in.IsDelim('}') {
		k := in.UnsafeBytes()
		in.WantColon()
		v := in.UnsafeBytes()

		buf = append(buf, model.Meta{Name: k, Value: v})

		in.WantComma()
	}
	in.Delim('}')
	in.Consumed()

	return buf, in.Error()
}

type options struct {
	filter func(k []byte) (allowed bool)
}

func applyOptions(optFns ...Option) *options {
	opts := &options{
		filter: func(k []byte) (allowed bool) { return true },
	}

	for _, o := range optFns {
		o(opts)
	}

	return opts
}

type Option func(*options)

func WithFilter(filter func(k []byte) (allowed bool)) Option {
	return func(o *options) { o.filter = filter }
}
