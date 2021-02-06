package protocol2

import (
	"context"
	"fmt"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
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
}

func (s *serviceV1) start() {
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

			// send notifications
			for _, ch := range channels {
				ch <- status
			}
		case <-s.stopCh:
			return
		}
	}
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

		//var data []byte
		//var err error

		if req.Type == proto.HashRequest_BODIES {

		} else if req.Type == proto.HashRequest_RECEIPTS {

		}

		body, ok := s.store.GetBodyByHash(hash)
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
