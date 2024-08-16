package consts

import (
	"math"
	"time"
)

const (
	RecieveBufferSize   = 2048
	SendBatchTimeout    = time.Millisecond
	RecieveBatchTimeout = time.Millisecond

	DefaultInitialWindowSize = 65_535
	DefaultTimeout           = 11 * time.Second
	DefaultMaxFrameSize      = 16384 // DefaultMaxFrameSize - максимальная длина пейлоада фрейма в grpc. У http2 ограничение больше.
	DefaultMaxHeaderListSize = math.MaxUint32
)
