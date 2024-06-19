package frameheader

import (
	"encoding/binary"
	"fmt"
	"strconv"

	"golang.org/x/net/http2"
)

type FrameHeader []byte

func NewFrameHeader() FrameHeader { return make([]byte, 9) }

func (f FrameHeader) Fill(
	length int,
	t http2.FrameType,
	flags http2.Flags,
	streamID uint32,
) {
	_ = f[8]
	f[0] = byte(length >> 16)
	f[1] = byte(length >> 8)
	f[2] = byte(length)
	f[3] = byte(t)
	f[4] = byte(flags)
	f[5] = byte(streamID >> 24)
	f[6] = byte(streamID >> 16)
	f[7] = byte(streamID >> 8)
	f[8] = byte(streamID)
}

func (f FrameHeader) Length() int {
	_ = f[2]
	return (int(f[0])<<16 | int(f[1])<<8 | int(f[2]))
}

func (f FrameHeader) SetLength(l int) {
	_ = f[2]
	f[0] = byte(l >> 16)
	f[1] = byte(l >> 8)
	f[2] = byte(l)
}

func (f FrameHeader) Type() http2.FrameType     { return http2.FrameType(f[3]) }
func (f FrameHeader) SetType(t http2.FrameType) { f[3] = byte(t) }

func (f FrameHeader) Flags() http2.Flags        { return http2.Flags(f[4]) }
func (f FrameHeader) SetFlags(flag http2.Flags) { f[4] = byte(flag) }

func (f FrameHeader) StreamID() uint32 { return binary.BigEndian.Uint32(f[5:]) }
func (f FrameHeader) SetStreamID(streamID uint32) {
	_ = f[8]
	f[5] = byte(streamID >> 24)
	f[6] = byte(streamID >> 16)
	f[7] = byte(streamID >> 8)
	f[8] = byte(streamID)
}

func (f FrameHeader) String() string {
	streamID := f.StreamID()
	return f.Type().String() +
		"/ length=" + strconv.FormatUint(uint64(f.Length()), 10) +
		"/ streamID = " + strconv.FormatUint(uint64(streamID), 10) +
		"/ flags = " + fmt.Sprintf("%o", f.Flags())
}
