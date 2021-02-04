package protocol2

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
)

var _ proto.V1Server = &serviceV1{}

// serviceV1 is the GRPC server implementation for the v1 protocol
type serviceV1 struct {
	s *Syncer
}

// GetReceiptsByHash implements the V1Server interface
func (s *serviceV1) GetReceiptsByHash(ctx context.Context, req *proto.HashRequest) (*proto.Response, error) {
	return nil, nil
}

// GetBodiesByHash implements the V1Server interface
func (s *serviceV1) GetBodiesByHash(ctx context.Context, req *proto.HashRequest) (*proto.Response, error) {
	hashes, err := req.DecodeHashes()
	if err != nil {
		return nil, err
	}
	resp := &proto.Response{
		Objs: []*proto.Response_Component{},
	}
	for _, hash := range hashes {
		body, ok := s.s.blockchain.GetBodyByHash(hash)
		if ok {

		} else {
			fmt.Println(body)
		}
	}
	return resp, nil
}

const maxHeadersAmount = 190

// GetHeaders implements the V1Server interface
func (s *serviceV1) GetHeaders(ctx context.Context, req *proto.GetHeadersRequest) (*proto.Response, error) {
	if req.Number != 0 && req.Hash != "" {
		return nil, fmt.Errorf("cannot have both")
	}
	if req.Amount > maxHeadersAmount {
		req.Amount = maxHeadersAmount
	}

	var origin *types.Header
	var ok bool

	if req.Number != 0 {
		origin, ok = s.s.blockchain.GetHeaderByNumber(uint64(req.Number))
	} else {
		var hash types.Hash
		if err := hash.UnmarshalText([]byte(req.Hash)); err != nil {
			return nil, err
		}
		origin, ok = s.s.blockchain.GetHeaderByHash(hash)
	}

	if !ok {
		// return empty
		return &proto.Response{}, nil
	}

	skip := req.Skip + 1

	resp := &proto.Response{
		Objs: []*proto.Response_Component{},
	}

	// resp

	count := int64(1)
	for count < req.Amount {
		block := int64(origin.Number) + skip

		if block < 0 {
			break
		}
		origin, ok = s.s.blockchain.GetHeaderByNumber(uint64(block))
		if !ok {
			break
		}
		count++
		// resp
	}

	return resp, nil
}

// Watch implements the V1Server interface
func (s *serviceV1) Watch(*empty.Empty, proto.V1_WatchServer) error {
	return nil
}
