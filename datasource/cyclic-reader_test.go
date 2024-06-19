package datasource

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

type errReader struct {
	seekErr error
	readErr error
}

func (r errReader) Seek(int64, int) (int64, error) {
	return 0, r.seekErr
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.readErr
}

var errBrand = errors.New("brand error")

func TestCyclicReaderErrors(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	buf := make([]byte, 1024)

	cr := NewCyclicReader(errReader{nil, errBrand})
	_, err := cr.Read(buf)
	assert.ErrorIs(err, errBrand)

	cr = NewCyclicReader(errReader{errBrand, io.EOF})
	_, err = cr.Read(buf)
	assert.ErrorIs(err, errBrand)
}

func TestCyclicReader(t *testing.T) {
	t.Parallel()
	assert := assert.New(t)
	buf := make([]byte, 1024)

	in := []byte("1234567890")
	cr := NewCyclicReader(bytes.NewReader(in))

	n, err := cr.Read(buf)
	assert.NoError(err)
	assert.Equal(in, buf[:n])

	n, err = cr.Read(buf)
	assert.NoError(err)
	assert.Equal([]byte{}, buf[:n])

	n, err = cr.Read(buf)
	assert.NoError(err)
	assert.Equal(in, buf[:n])
}
