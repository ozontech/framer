package reciever_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"

	"github.com/ozontech/framer/loader/reciever"
)

func TestFramer(t *testing.T) {
	t.Parallel()
	a := assert.New(t)
	f := new(reciever.Framer)

	buf := bytes.NewBuffer(nil)
	framer := http2.NewFramer(buf, nil)

	payload1 := make([]byte, 512)
	_, err := rand.Read(payload1)
	a.NoError(err)
	a.NoError(framer.WriteData(123, false, payload1))
	firstFrameLen := buf.Len()

	payload2 := make([]byte, 512)
	_, err = rand.Read(payload2)
	a.NoError(err)
	a.NoError(framer.WriteData(321, true, payload2))

	p := buf.Bytes()
	f.Fill(p[:1])
	b, status := f.Next()
	a.Nil(b)
	a.Equal(reciever.StatusHeaderIncomplete, status)

	f.Fill(p[1:9])
	b, status = f.Next()
	a.Empty(b)
	a.Equal(reciever.StatusPayloadIncomplete, status)

	header := f.Header()
	a.Equal(512, header.Length())
	a.Equal(http2.FrameData, header.Type())
	a.Equal(http2.Flags(0), header.Flags())
	a.Equal(uint32(123), header.StreamID())

	f.Fill(p[9:11])
	b, status = f.Next()
	a.Equal(b, p[9:11])
	a.Equal(reciever.StatusPayloadIncomplete, status)

	f.Fill(p[11 : firstFrameLen+15])
	b, status = f.Next()
	a.Equal(b, p[11:firstFrameLen])
	a.Equal(reciever.StatusFrameDone, status)

	b, status = f.Next()
	a.Equal(b, p[firstFrameLen+9:firstFrameLen+15])
	a.Equal(reciever.StatusPayloadIncomplete, status)

	f.Fill(p[firstFrameLen+15:])
	b, status = f.Next()
	a.Equal(b, p[firstFrameLen+15:])
	a.Equal(reciever.StatusFrameDoneBufEmpty, status)
}
