package protocol

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	defaultBodyFetchTimeout = time.Second * 10
)

var (
	errMalformedHeadersResponse = errors.New("malformed headers response")
	errMalformedHeadersBody     = errors.New("malformed headers body")
	errInvalidHeaderSequence    = errors.New("invalid header sequence")
	errHeaderBodyMismatch       = errors.New("requested body and header mismatch")
)

func getHeaders(clt proto.V1Client, req *proto.GetHeadersRequest) ([]*types.Header, error) {
	resp, err := clt.GetHeaders(context.Background(), req)
	if err != nil {
		return nil, err
	}

	headers := make([]*types.Header, len(resp.Objs))

	for index, obj := range resp.Objs {
		// Verify the header response is correctly formed
		if verifyErr := verifyHeadersResponse(obj); verifyErr != nil {
			return nil, fmt.Errorf(
				"unable to verify headers response, %w",
				verifyErr,
			)
		}

		header := &types.Header{}
		if err := header.UnmarshalRLP(obj.Spec.Value); err != nil {
			return nil, err
		}

		headers[index] = header
	}

	return headers, nil
}

// verifyHeadersResponse verifies the headers response to the peer
func verifyHeadersResponse(headersResp *proto.Response_Component) error {
	// Make sure the header response is present
	if headersResp == nil {
		return errMalformedHeadersResponse
	}

	// Make sure the header body is present
	if headersResp.Spec == nil {
		return errMalformedHeadersBody
	}

	return nil
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
			return errInvalidHeaderSequence
		}
	}

	// Construct the body request
	headerHashes := make([]types.Hash, len(headers))
	for index, header := range headers {
		headerHashes[index] = header.Hash
	}

	getBodiesContext, cancelFn := context.WithTimeout(
		context.Background(),
		defaultBodyFetchTimeout,
	)
	defer cancelFn()

	// Grab the block bodies
	bodies, err := getBodies(getBodiesContext, peerClient, headerHashes)
	if err != nil {
		return err
	}

	if len(bodies) != len(headers) {
		return errHeaderBodyMismatch
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
