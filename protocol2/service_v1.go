package protocol2

import (
	"context"
	"fmt"
	"sync"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/go-hclog"
)

var _ proto.V1Server = &serviceV1{}

// serviceV1 is the GRPC server implementation for the v1 protocol
type serviceV1 struct {
	logger hclog.Logger

	store Blockchain
	subs  blockchain.Subscription

	addCh  chan chan *proto.V1Status
	stopCh chan struct{}

	status     *proto.V1Status
	statusLock sync.Mutex
}

func (s *serviceV1) start() {
	// get the current status of the syncer
	currentHeader := s.store.Header()

	diff, ok := s.store.GetTD(currentHeader.Hash)
	if !ok {
		panic("Failed to read difficulty")
	}

	s.status = &proto.V1Status{
		Hash:       currentHeader.Hash.String(),
		Number:     int64(currentHeader.Number),
		Difficulty: diff.String(),
	}

	eventCh := s.subs.GetEventCh()

	s.addCh = make(chan chan *proto.V1Status, 10)
	s.stopCh = make(chan struct{})

	channels := make([]chan *proto.V1Status, 0)

	// watch the subscription and notify
	for {
		select {
		case add := <-s.addCh:
			channels = append(channels, add)

		case evnt := <-eventCh:
			if evnt.Type == blockchain.EventFork {
				// we do not want to notify forks
				continue
			}
			if len(evnt.NewChain) == 0 {
				// this should not happen
				continue
			}

			status := &proto.V1Status{
				Difficulty: evnt.Difficulty.String(),
				Hash:       evnt.NewChain[0].Hash.String(),
				Number:     int64(evnt.NewChain[0].Number),
			}

			s.statusLock.Lock()
			s.status = status
			s.statusLock.Unlock()

			// send notifications
			for _, ch := range channels {
				ch <- status
			}
		case <-s.stopCh:
			return
		}
	}
}

type rlpObject interface {
	MarshalRLPTo(dst []byte) []byte
	UnmarshalRLP(input []byte) error
}

// GetCurrent implements the V1Server interface
func (s *serviceV1) GetCurrent(ctx context.Context, in *empty.Empty) (*proto.V1Status, error) {
	s.statusLock.Lock()
	status := s.status
	s.statusLock.Unlock()
	return status, nil
}

// GetObjectsByHash implements the V1Server interface
func (s *serviceV1) GetObjectsByHash(ctx context.Context, req *proto.HashRequest) (*proto.Response, error) {
	hashes, err := req.DecodeHashes()
	if err != nil {
		return nil, err
	}
	resp := &proto.Response{
		Objs: []*proto.Response_Component{},
	}
	for _, hash := range hashes {
		var obj rlpObject
		var found bool

		if req.Type == proto.HashRequest_BODIES {
			obj, found = s.store.GetBodyByHash(hash)
		} else if req.Type == proto.HashRequest_RECEIPTS {
			var raw []*types.Receipt
			raw, found = s.store.GetReceiptsByHash(hash)
			obj = types.Receipts(raw)
		}

		var data []byte
		if found {
			data = obj.MarshalRLPTo(nil)
		} else {
			data = []byte{}
		}

		resp.Objs = append(resp.Objs, &proto.Response_Component{
			Spec: &any.Any{
				Value: data,
			},
		})
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

	// resp

	count := int64(1)
	for count < req.Amount {
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
	}

	return resp, nil
}

// Watch implements the V1Server interface
func (s *serviceV1) Watch(req *empty.Empty, stream proto.V1_WatchServer) error {
	watchCh := make(chan *proto.V1Status)
	s.addCh <- watchCh

	for {
		select {
		case status := <-watchCh:
			if err := stream.Send(status); err != nil {
				s.logger.Error("[ERROR]: failed to send watch status", "err", err)
				close(watchCh)
				return nil
			}

		case <-stream.Context().Done():
			close(watchCh)
			return nil
		}
	}
}

// Helper functions to decode responses from the grpc layer
func getBodies(ctx context.Context, clt proto.V1Client, hashes []types.Hash) ([]*types.Body, error) {
	input := []string{}
	for _, h := range hashes {
		input = append(input, h.String())
	}
	resp, err := clt.GetObjectsByHash(ctx, &proto.HashRequest{Hash: input, Type: proto.HashRequest_BODIES})
	if err != nil {
		return nil, err
	}
	res := []*types.Body{}
	for _, obj := range resp.Objs {
		var body types.Body
		if obj.Spec.Value != nil {
			if err := body.UnmarshalRLP(obj.Spec.Value); err != nil {
				return nil, err
			}
		}
		res = append(res, &body)
	}
	return res, nil
}

func getReceipts(ctx context.Context, clt proto.V1Client, hashes []types.Hash) ([]*types.Receipts, error) {
	input := []string{}
	for _, h := range hashes {
		input = append(input, h.String())
	}
	resp, err := clt.GetObjectsByHash(ctx, &proto.HashRequest{Hash: input, Type: proto.HashRequest_RECEIPTS})
	if err != nil {
		return nil, err
	}
	res := []*types.Receipts{}
	for _, obj := range resp.Objs {
		var receipts types.Receipts
		if obj.Spec.Value != nil {
			if err := receipts.UnmarshalRLP(obj.Spec.Value); err != nil {
				return nil, err
			}
		}
		res = append(res, &receipts)
	}
	return res, nil
}
