//go:generate moq -pkg reciever -out ./mock_loader_types_test.go ../types StreamStore Stream FlowControl
//go:generate moq -pkg reciever -fmt goimports -out ./mock_reciever_test.go . FrameTypeProcessor
package reciever
