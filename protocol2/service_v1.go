package protocol2

import "github.com/0xPolygon/minimal/protocol2/proto"

var _ proto.V1Server = &serviceV1{}

// serviceV1 is the GRPC server implementation for the v1 protocol
type serviceV1 struct {
}

func (s *serviceV1) WatchBlocks(req *proto.Empty, stream proto.V1_WatchBlocksServer) error {
	return nil
}
