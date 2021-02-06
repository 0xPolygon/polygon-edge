package minimal

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/protobuf/types/known/emptypb"
)

type handshakeService struct {
}

func (h *handshakeService) Hello(ctx context.Context, req *empty.Empty) (*empty.Empty, error) {
	return &emptypb.Empty{}, nil
}
