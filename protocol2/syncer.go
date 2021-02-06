package protocol2

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"
)

// Blockchain is the interface required by the syncer to connect to the blockchain
type Blockchain interface {
	SubscribeEvents() blockchain.Subscription
	Header() *types.Header
	GetReceiptsByHash(types.Hash) []*types.Receipt
	GetBodyByHash(types.Hash) (*types.Body, bool)
	GetHeaderByHash(types.Hash) (*types.Header, bool)
	GetHeaderByNumber(n uint64) (*types.Header, bool)
}

// Syncer is a sync protocol
type Syncer struct {
	blockchain Blockchain

	peersLock sync.Mutex
	peers     map[string]*peer

	stopCh chan struct{}
}

type peer struct {
	client *proto.V1Client
}

func NewSyncer() *Syncer {
	return &Syncer{
		peers:  map[string]*peer{},
		stopCh: make(chan struct{}),
	}
}

func (s *Syncer) Register(server *grpc.Server) {
	proto.RegisterV1Server(server, &serviceV1{})
}

func (s *Syncer) Start() {
	go s.run()
}

func (s *Syncer) HandleUser(conn *grpc.ClientConn) {
	fmt.Println("- new user -")

	// watch for changes of the other node first
	clt := proto.NewV1Client(conn)

	stream, err := clt.Watch(context.Background(), &empty.Empty{})
	if err != nil {
		panic(err)
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			panic(err)
		}
		fmt.Println("-- msg --")
		fmt.Println(msg)
	}
}

func (s *Syncer) findCommonAncestor(clt proto.V1Client, height *types.Header) (*types.Header, error) {
	h := s.blockchain.Header()

	min := uint64(0) // genesis
	max := h.Number

	if heightNumber := height.Number; max > heightNumber {
		max = heightNumber
	}

	var header *types.Header
	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		req := &proto.GetHeadersRequest{
			Number: int64(m),
		}
		resp, err := clt.GetHeaders(context.Background(), req)
		if err != nil {
			return nil, err
		}

		if len(resp.Objs) == 0 {
			// peer does not have the m peer, search in lower bounds
			max = m - 1
		} else {
			if len(resp.Objs) != 1 {
				return nil, fmt.Errorf("unexpected more than 1 result")
			}

			var found *types.Header
			if found.Number != m {
				return nil, fmt.Errorf("header response number not correct, asked %d but retrieved %d", m, header.Number)
			}

			expectedHeader, ok := s.blockchain.GetHeaderByNumber(m)
			if !ok {
				return nil, fmt.Errorf("cannot find the header %d in local chain", m)
			}
			if expectedHeader.Hash == found.Hash {
				header = found
				min = m + 1
			} else {
				if m == 0 {
					return nil, fmt.Errorf("genesis does not match?")
				}
				max = m - 1
			}
		}
	}

	if min == 0 {
		return nil, nil
	}
	return header, nil
}

func (s *Syncer) run() {
	var cancelFn context.CancelFunc

	for {
		select {
		case <-s.stopCh:
			if cancelFn != nil {
				cancelFn()
			}
		}
	}
}
