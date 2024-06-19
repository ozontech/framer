package main

import (
	"io"
	"log"
	"os"
	"strconv"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb" // here must be your generated protobuf
)

type requestsWriter struct {
	w   io.Writer
	buf []byte
}

func newRequestsWriter(w io.Writer) *requestsWriter {
	return &requestsWriter{w, make([]byte, 0, 1024)}
}

func (w *requestsWriter) WriteRequest(tag, path string, message []byte) error {
	w.buf = w.buf[:10]
	w.buf = append(w.buf, '\n')
	w.buf = append(w.buf, []byte(path)...)
	w.buf = append(w.buf, '\n')
	w.buf = append(w.buf, []byte(tag)...)
	w.buf = append(w.buf, '\n')
	w.buf = append(w.buf, message...)
	w.buf = append(w.buf, '\n')

	l := []byte(strconv.Itoa(len(w.buf) - 10))
	b := w.buf[10-len(l):]
	copy(b, l)
	_, err := w.w.Write(b)
	return err
}

func run(w *requestsWriter) error {
	for i := 0; i < 10_000; i++ {
		pl, err := proto.Marshal(wrapperspb.String("i am your request #" + strconv.Itoa(i)))
		if err != nil {
			return err
		}
		err = w.WriteRequest("/my.service/Method", "request tag", pl)
		if err != nil {
			return err
		}
	}
	return nil
}

func main() {
	err := run(newRequestsWriter(os.Stdout))
	if err != nil {
		log.Fatal(err.Error())
	}
}
