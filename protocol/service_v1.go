package protocol

import (
	"context"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/network/grpc"
	"github.com/0xPolygon/polygon-edge/protocol/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/libp2p/go-libp2p-core/peer"
	anypb "google.golang.org/protobuf/types/known/anypb"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

// serviceV1 is the GRPC server implementation for the v1 protocol
type serviceV1 struct {
	proto.UnimplementedV1Server

	syncer *Syncer
	logger hclog.Logger

	store blockchainShim
}

type rlpObject interface {
	MarshalRLPTo(dst []byte) []byte
	UnmarshalRLP(input []byte) error
}

var (
	errInvalidHeadersRequest  = errors.New("invalid headers request")
	errMalformedNotifyRequest = errors.New("malformed notify request")
	errMalformedNotifyBody    = errors.New("malformed notify body")
	errMalformedNotifyStatus  = errors.New("malformed notify status")
)

func (s *serviceV1) Notify(ctx context.Context, req *proto.NotifyReq) (*empty.Empty, error) {
	var id peer.ID

	if ctx, ok := ctx.(*grpc.Context); ok {
		id = ctx.PeerID
	} else {
		return &empty.Empty{}, nil
	}

	// Do the initial notify request verification
	if verifyErr := verifyNotifyRequest(req); verifyErr != nil {
		return nil, fmt.Errorf("unable to verify notify request, %w", verifyErr)
	}

	b := new(types.Block)
	if err := b.UnmarshalRLP(req.Raw.Value); err != nil {
		return nil, err
	}

	status, err := statusFromProto(req.Status)

	if err != nil {
		return nil, err
	}

	s.syncer.enqueueBlock(id, b)
	s.syncer.updatePeerStatus(id, status)

	return &empty.Empty{}, nil
}

// verifyNotifyRequest verifies the notify request to the peer
func verifyNotifyRequest(request *proto.NotifyReq) error {
	// Make sure the request is formed
	if request == nil {
		return errMalformedNotifyRequest
	}

	// Make sure the notify body (block) is present
	if request.Raw == nil {
		return errMalformedNotifyBody
	}

	// Make sure the notify status is present
	if request.Status == nil {
		return errMalformedNotifyStatus
	}

	return nil
}

// GetCurrent implements the V1Server interface
func (s *serviceV1) GetCurrent(_ context.Context, _ *empty.Empty) (*proto.V1Status, error) {
	return s.syncer.status.toProto(), nil
}

// GetObjectsByHash implements the V1Server interface
func (s *serviceV1) GetObjectsByHash(_ context.Context, req *proto.HashRequest) (*proto.Response, error) {
	hashes, err := req.DecodeHashes()
	if err != nil {
		return nil, err
	}

	resp := &proto.Response{
		Objs: []*proto.Response_Component{},
	}

	for _, hash := range hashes {
		var obj rlpObject

		if req.Type == proto.HashRequest_BODIES {
			obj, _ = s.store.GetBodyByHash(hash)
		} else if req.Type == proto.HashRequest_RECEIPTS {
			var raw []*types.Receipt
			raw, err = s.store.GetReceiptsByHash(hash)
			if err != nil {
				return nil, err
			}

			receipts := types.Receipts(raw)
			obj = &receipts
		}

		var data []byte
		if obj != nil {
			data = obj.MarshalRLPTo(nil)
		} else {
			data = []byte{}
		}

		resp.Objs = append(resp.Objs, &proto.Response_Component{
			Spec: &anypb.Any{
				Value: data,
			},
		})
	}

	return resp, nil
}

const MaxSkeletonHeadersAmount = 190

// GetHeaders implements the V1Server interface
func (s *serviceV1) GetHeaders(_ context.Context, req *proto.GetHeadersRequest) (*proto.Response, error) {
	if req.Number != 0 && req.Hash != "" {
		return nil, errInvalidHeadersRequest
	}

	if req.Amount > MaxSkeletonHeadersAmount {
		req.Amount = MaxSkeletonHeadersAmount
	}

	var (
		origin *types.Header
		ok     bool
	)

	if req.Number != 0 {
		origin, ok = s.store.GetHeaderByNumber(uint64(req.Number))
	} else {
		var hash types.Hash
		if err := hash.UnmarshalText([]byte(req.Hash)); err != nil {
			return nil, err
		}
		origin, ok = s.store.GetHeaderByHash(hash)
	}

	if !ok {
		// return empty
		return &proto.Response{}, nil
	}

	skip := req.Skip + 1

	resp := &proto.Response{
		Objs: []*proto.Response_Component{},
	}

	addData := func(h *types.Header) {
		resp.Objs = append(resp.Objs, &proto.Response_Component{
			Spec: &anypb.Any{
				Value: h.MarshalRLPTo(nil),
			},
		})
	}

	// resp
	addData(origin)

	for count := int64(1); count < req.Amount; {
		block := int64(origin.Number) + skip

		if block < 0 {
			break
		}

		origin, ok = s.store.GetHeaderByNumber(uint64(block))

		if !ok {
			break
		}
		count++

		// resp
		addData(origin)
	}

	return resp, nil
}

// Helper functions to decode responses from the grpc layer
func getBodies(ctx context.Context, clt proto.V1Client, hashes []types.Hash) ([]*types.Body, error) {
	input := make([]string, 0, len(hashes))

	for _, h := range hashes {
		input = append(input, h.String())
	}

	resp, err := clt.GetObjectsByHash(
		ctx,
		&proto.HashRequest{
			Hash: input,
			Type: proto.HashRequest_BODIES,
		},
	)
	if err != nil {
		return nil, err
	}

	res := make([]*types.Body, 0, len(resp.Objs))

	for _, obj := range resp.Objs {
		var body types.Body
		if obj.Spec.Value != nil {
			if err := body.UnmarshalRLP(obj.Spec.Value); err != nil {
				return nil, err
			}
		}

		res = append(res, &body)
	}

	if len(res) != len(input) {
		return nil, fmt.Errorf("not correct size")
	}

	return res, nil
}
