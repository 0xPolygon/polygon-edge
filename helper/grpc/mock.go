package grpc

import (
	"context"

	"google.golang.org/grpc/metadata"
)

type MockGrpcClientStream struct {
}

func (mk *MockGrpcClientStream) Header() (metadata.MD, error) {
	return nil, nil
}

func (mk *MockGrpcClientStream) Trailer() metadata.MD {
	return nil
}

func (mk *MockGrpcClientStream) CloseSend() error {
	return nil
}

func (mk *MockGrpcClientStream) Context() context.Context {
	return context.Background()
}

func (mk *MockGrpcClientStream) SendMsg(m interface{}) error {
	return nil
}

func (mk *MockGrpcClientStream) RecvMsg(m interface{}) error {
	return nil
}
