package jsonkv

import (
	"encoding/json"
	"testing"

	"github.com/ozontech/framer/formats/model"
)

// BenchmarkSingleValEncoding-8         	 9681330	       126.0 ns/op	       0 B/op	       0 allocs/op
// BenchmarkLegacySingleValEncoding-8   	 1222810	      1631 ns/op	     936 B/op	      15 allocs/op
// BenchmarkSingleValJsonKVDecoding-8   	 3303810	       364.9 ns/op	       0 B/op	       0 allocs/op
// BenchmarkLegacySingleValDecoding-8   	  357715	      3251 ns/op	     792 B/op	      26 allocs/op

func BenchmarkSingleValEncoding(b *testing.B) {
	headers := []model.Meta{
		{Name: []byte("header1"), Value: []byte("value1")},
		{Name: []byte("header2"), Value: []byte("value2")},
		{Name: []byte("header3"), Value: []byte("value3")},
		{Name: []byte("header4"), Value: []byte("value4")},
	}

	e := NewSingleVal()
	bb := make([]byte, 0, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bb = e.MarshalAppend(bb, headers)
	}
}

func BenchmarkLegacySingleValEncoding(b *testing.B) {
	s := []string{
		"header1", "value1",
		"header2", "value2",
		"header3", "value3",
		"header4", "value4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := make(map[string]string, len(s)/2)
		for i := 0; i < len(s); i += 2 {
			m[s[i]] = s[i+1]
		}
		_, err := json.Marshal(m)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSingleValJsonKVDecoding(b *testing.B) {
	in := []byte(`{ "header1": "value1", "header2": "value2", "header3": "value3", "header4": "value4", "header5": "value5" }`)
	d := SingleVal{}
	buf := make([]model.Meta, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var err error
		buf, err = d.UnmarshalAppend(buf[:0], in)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLegacySingleValDecoding(b *testing.B) {
	in := []byte(`{ "header1": "value1", "header2": "value2", "header3": "value3", "header4": "value4", "header5": "value5" }`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		headers := make(map[string]string)
		err := json.Unmarshal(in, &headers)
		if err != nil {
			b.Fatal(err)
		}
	}
}
