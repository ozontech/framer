package io

import (
	"bytes"
	"io"
)

type Reader struct {
	buf         [4096]byte
	unprocessed []byte

	r io.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) fillUnprocessed() error {
	n, err := r.r.Read(r.buf[:])
	if err != nil {
		return err
	}
	r.unprocessed = r.buf[:n]
	return nil
}

// ReadNext использует p как буфер для чтения, при необходимости выделяя новую память.
func (r *Reader) ReadNext(p []byte) ([]byte, error) {
	for {
		if len(r.unprocessed) == 0 {
			err := r.fillUnprocessed()
			if err == io.EOF && len(p) > 0 {
				return p, nil
			}
			if err != nil {
				return p, err
			}
		}

		index := bytes.IndexByte(r.unprocessed, '\n')
		if index != -1 {
			p = append(p, r.unprocessed[:index]...)
			r.unprocessed = r.unprocessed[index+1:]
			return p, nil
		}

		p = append(p, r.unprocessed...)
		r.unprocessed = nil
	}
}

type Writer struct {
	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) WriteNext(p []byte) error {
	_, err := w.w.Write(append(p, '\n'))
	return err
}
