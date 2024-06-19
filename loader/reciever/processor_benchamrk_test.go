package reciever

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/http2"
)

func BenchmarkProcessor(b *testing.B) {
	a := assert.New(b)
	priorityFramesChan := make(chan []byte, 100)
	bytesChan := make(chan []byte)

	processor := Processor{
		new(Framer), []FrameTypeProcessor{
			http2.FramePing: newPingFrameProcessor(priorityFramesChan),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.NoError(processor.Run(bytesChan))
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(bytesChan)
				return
			case <-priorityFramesChan:
			}
		}
	}()

	bufW := bytes.NewBuffer(nil)
	framer := http2.NewFramer(bufW, nil)
	pingPayload := [8]byte{8, 7, 6, 5, 4, 3, 2, 1}

	a.NoError(framer.WritePing(false, pingPayload))
	frameLen := bufW.Len()
	for i := 0; i < 10; i++ {
		a.NoError(framer.WritePing(false, pingPayload))
	}

	bts := bufW.Bytes()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b := bts[:]
		for len(b) > 0 {
			cutIndex := min(len(b), frameLen+1)
			bytesChan <- b[:cutIndex]
			b = b[cutIndex:]
		}
	}

	cancel()
	<-done
}
