package jsonkv

import (
	"encoding/json"
	"testing"

	"github.com/mailru/easyjson/jlexer"
	"github.com/ozontech/framer/formats/model"
)

// BenchmarkMultiValEncoding-8          	 6392497	       180.7 ns/op	      24 B/op	       1 allocs/op
// BenchmarkLegacyMultiValEncoding-8    	  908154	      1183 ns/op	    1112 B/op	      19 allocs/op
// BenchmarkMultiValJsonDecoding-8      	 4344513	       277.3 ns/op	       0 B/op	       0 allocs/op
// BenchmarkLegacyMultiValDecoding-8    	 1857330	       641.0 ns/op	     560 B/op	      14 allocs/op

func BenchmarkMultiValEncoding(b *testing.B) {
	meta := []model.Meta{
		{Name: []byte("header1"), Value: []byte("value1")},
		{Name: []byte("header2"), Value: []byte("value2")},
		{Name: []byte("header3"), Value: []byte("value3")},
		{Name: []byte("header4"), Value: []byte("value4")},
	}

	encoder := NewMultiVal()
	buf := make([]byte, 0, 1024)
	for i := 0; i < b.N; i++ {
		buf = encoder.MarshalAppend(buf[:0], meta)
	}
}

func BenchmarkLegacyMultiValEncoding(b *testing.B) {
	s := []string{
		"header1", "value1",
		"header2", "value2",
		"header3", "value3",
		"header4", "value4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := make(map[string][]string, len(s)/2)
		for i := 0; i < len(s); i += 2 {
			m[s[i]] = append(m[s[i]], s[i+1])
		}
		_, err := json.Marshal(m)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultiValJsonDecoding(b *testing.B) {
	in := []byte(`{ "header1": ["value1"], "header2": ["value2"], "header3": ["value3"], "header4": ["value4"], "header5": ["value5"] }`)
	decoder := MultiVal{}
	buf := make([]model.Meta, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = decoder.UnmarshalAppend(buf[:0], in)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLegacyMultiValDecoding(b *testing.B) {
	in := []byte(`{ "header1": ["value1"], "header2": ["value2"], "header3": ["value3"], "header4": ["value4"], "header5": ["value5"] }`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := unmarshalJSONMeta(in)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func unmarshalJSONMeta(b []byte) ([]string, error) {
	r := []string{}

	in := jlexer.Lexer{Data: b}

	in.Delim('{')
	for !in.IsDelim('}') {
		// key := in.String()
		key := in.String()

		in.WantColon()

		in.Delim('[')
		for !in.IsDelim(']') {
			r = append(r, key, in.String())
			in.WantComma()
		}
		in.Delim(']')

		in.WantComma()
	}
	in.Delim('}')
	in.Consumed()

	return r, in.Error()
}
