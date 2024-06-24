package consts

import "time"

const (
	ChunksBufferSize    = 2048
	RecieveBufferSize   = 2048
	SendBatchTimeout    = time.Millisecond
	RecieveBatchTimeout = time.Millisecond

	DefaultInitialWindowSize = 65_535
	DefaultTimeout           = 11 * time.Second
	DefaultMaxFrameSize      = 16384 // Максимальная длина пейлоада фрейма в grpc. У http2 ограничение больше.
)
