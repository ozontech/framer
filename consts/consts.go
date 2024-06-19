package consts

import "time"

const (
	ChunksBufferSize    = 2048
	RecieveBufferSize   = 2048
	SendBatchTimeout    = time.Millisecond
	RecieveBatchTimeout = time.Millisecond
)
