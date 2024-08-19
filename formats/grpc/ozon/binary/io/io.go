package io

import (
	"fmt"
	"io"
	"strconv"
)

const nChar = '\n'

type Reader struct {
	buf         []byte
	unprocessed []byte

	r io.Reader
}

func NewReader(r io.Reader, bufSize ...int) *Reader {
	size := 4096
	if len(bufSize) > 0 {
		size = bufSize[0]
	}
	return &Reader{
		buf: make([]byte, size),
		r:   r,
	}
}

func (r *Reader) fillUnprocessed() error {
	n, err := r.r.Read(r.buf[:])
	if err != nil {
		return err
	}
	r.unprocessed = r.buf[:n]
	return nil
}

// ReadNext читает контейнер запроса
func (r *Reader) ReadNext(b []byte) ([]byte, error) {
	var (
		size int
		n    int
		err  error
	)

	for {
		size, n, err = fillSize(size, r.unprocessed)
		r.unprocessed = r.unprocessed[n:]
		if err == nil {
			break
		}

		if err != io.ErrUnexpectedEOF {
			return nil, err
		}

		err = r.fillUnprocessed()
		if err != nil {
			return nil, err
		}
	}

	if cap(b) < size {
		b = append(b, make([]byte, size-len(b))...)
	} else {
		b = b[:size]
	}
	var filled int
	for filled < size {
		if len(r.unprocessed) == 0 {
			err = r.fillUnprocessed()
			if err != nil {
				return nil, err
			}
		}

		n = copy(b[filled:], r.unprocessed)
		filled += n
		r.unprocessed = r.unprocessed[n:]
	}

	for {
		if len(r.unprocessed) == 0 {
			err = r.fillUnprocessed()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
		} else {
			if r.unprocessed[0] != nChar {
				break
			}
			r.unprocessed = r.unprocessed[1:]
		}
	}

	return b, nil
}

func fillSize(accIn int, in []byte) (acc int, consumed int, err error) {
	for i, b := range in {
		var d int
		switch {
		case '0' <= b && b <= '9':
			d = int(b - '0')
		case b == '\n':
			return accIn, i + 1, nil
		default:
			return accIn, i + 1, fmt.Errorf("unexpected char: '%c'", b)
		}
		accIn = accIn*10 + d
	}
	return accIn, len(in), io.ErrUnexpectedEOF
}

type Writer struct {
	w         io.Writer
	prefixBuf []byte
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

func (w *Writer) WriteNext(p []byte) error {
	w.prefixBuf = strconv.AppendUint(w.prefixBuf[:0], uint64(len(p)), 10)
	w.prefixBuf = append(w.prefixBuf, '\n')
	_, err := w.w.Write(w.prefixBuf)
	if err != nil {
		return err
	}

	p = append(p, '\n')
	_, err = w.w.Write(p)
	if err != nil {
		return err
	}

	return nil
}
