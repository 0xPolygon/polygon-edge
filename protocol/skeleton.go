package protocol

import (
	"context"
	"errors"
	"time"

	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

func getHeaders(clt proto.V1Client, req *proto.GetHeadersRequest) ([]*types.Header, error) {
	resp, err := clt.GetHeaders(context.Background(), req)
	if err != nil {
		return nil, err
	}

	headers := make([]*types.Header, 0)

	for _, obj := range resp.Objs {
		header := &types.Header{}
		if err := header.UnmarshalRLP(obj.Spec.Value); err != nil {
			return nil, err
		}

		headers = append(headers, header)
	}

	return headers, nil
}

type skeleton struct {
	blocks []*types.Block
	skip   int64
	amount int64
}

// getBlocksFromPeer fetches the blocks from the peer,
// from the specified block number (including)
func (s *skeleton) getBlocksFromPeer(
	peerClient proto.V1Client,
	initialBlockNum uint64,
) error {
	// Fetch the headers from the peer
	headers, err := getHeaders(
		peerClient,
		&proto.GetHeadersRequest{
			Number: int64(initialBlockNum),
			Skip:   s.skip,
			Amount: s.amount,
		},
	)
	if err != nil {
		return err
	}

	// Make sure the number sequences match up
	for i := 1; i < len(headers); i++ {
		if headers[i].Number-headers[i-1].Number != 1 {
			return errors.New("invalid header sequence")
		}
	}

	// Construct the body request
	headerHashes := make([]types.Hash, len(headers))
	for index, header := range headers {
		headerHashes[index] = header.Hash
	}

	getBodiesContext, cancelFn := context.WithTimeout(
		context.Background(),
		time.Second*10,
	)
	defer cancelFn()

	// Grab the block bodies
	bodies, err := getBodies(getBodiesContext, peerClient, headerHashes)
	if err != nil {
		return err
	}

	if len(bodies) != len(headers) {
		return errors.New("requested body and header mismatch")
	}

	s.blocks = make([]*types.Block, len(headers))

	for index, body := range bodies {
		s.blocks[index] = &types.Block{
			Header:       headers[index],
			Transactions: body.Transactions,
		}
	}

	return nil
}
