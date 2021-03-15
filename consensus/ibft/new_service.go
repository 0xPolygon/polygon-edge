package ibft

import (
	"context"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/golang/protobuf/ptypes/empty"
)

type service struct {
	proto.UnimplementedIbftServer

	b *Backend2
}

func (s *service) Prepare(ctx context.Context, req *proto.Subject) (*empty.Empty, error) {
	return nil, nil
}

func (s *service) PrePrepare(ctx context.Context, req *proto.Preprepare) (*empty.Empty, error) {
	return nil, nil
}

func (s *service) Commit(ctx context.Context, req *proto.Subject) (*empty.Empty, error) {
	return nil, nil
}

func (s *service) RoundChange(ctx context.Context, req *proto.Subject) (*empty.Empty, error) {
	return nil, nil
}
