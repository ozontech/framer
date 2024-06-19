package datasource

import (
	"fmt"
	"io"
)

type CyclicReader struct {
	rs io.ReadSeeker
}

func NewCyclicReader(rs io.ReadSeeker) *CyclicReader {
	return &CyclicReader{rs}
}

func (r *CyclicReader) Read(b []byte) (int, error) {
	n, err := r.rs.Read(b)
	if err == nil {
		return n, nil
	}

	if err != io.EOF {
		return n, fmt.Errorf("read: %w", err)
	}

	_, err = r.rs.Seek(0, io.SeekStart)
	if err != nil {
		return n, fmt.Errorf("rewind: %w", err)
	}

	return n, nil
}
