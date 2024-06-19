package reciever

import "github.com/ozontech/framer/frameheader"

type Framer struct {
	currentHeader frameheader.FrameHeader
	header        frameheader.FrameHeader
	payloadLeft   int
	buf           []byte
}

type Status int

const (
	StatusFrameDone Status = iota
	StatusFrameDoneBufEmpty
	StatusHeaderIncomplete
	StatusPayloadIncomplete
)

func (p *Framer) Header() frameheader.FrameHeader {
	return p.header
}

func (p *Framer) Next() ([]byte, Status) {
	currentHeaderLen := len(p.currentHeader)
	if currentHeaderLen != 9 {
		bufLen := len(p.buf)
		needToFill := 9 - currentHeaderLen
		if bufLen < needToFill {
			p.currentHeader = append(p.currentHeader, p.buf...)
			return nil, StatusHeaderIncomplete
		}

		p.currentHeader = append(p.currentHeader, p.buf[:needToFill]...)
		p.buf = p.buf[needToFill:]
		p.payloadLeft = p.currentHeader.Length()
	}
	p.header = p.currentHeader

	bufLen := len(p.buf)
	if bufLen > p.payloadLeft {
		payload := p.buf[:p.payloadLeft]
		p.buf = p.buf[p.payloadLeft:]
		p.currentHeader = p.currentHeader[:0]
		return payload, StatusFrameDone
	}

	if bufLen == p.payloadLeft {
		p.currentHeader = p.currentHeader[:0]
		return p.buf, StatusFrameDoneBufEmpty
	}

	p.payloadLeft -= len(p.buf)
	return p.buf, StatusPayloadIncomplete
}

func (p *Framer) Fill(b []byte) {
	p.buf = b
}
