package protocol2

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"sync"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Blockchain is the interface required by the syncer to connect to the blockchain
type Blockchain interface {
	SubscribeEvents() blockchain.Subscription
	Header() *types.Header
	GetTD(hash types.Hash) (*big.Int, bool)
	GetReceiptsByHash(types.Hash) ([]*types.Receipt, bool)
	GetBodyByHash(types.Hash) (*types.Body, bool)
	GetHeaderByHash(types.Hash) (*types.Header, bool)
	GetHeaderByNumber(n uint64) (*types.Header, bool)
}

type peer struct {
	id     string
	client *proto.V1Client
}

// Syncer is a sync protocol
type Syncer struct {
	blockchain Blockchain

	peersLock sync.Mutex
	peers     map[string]*peer

	serviceV1 *serviceV1
	stopCh    chan struct{}
}

func NewSyncer(blockchain Blockchain) *Syncer {
	return &Syncer{
		peers:      map[string]*peer{},
		stopCh:     make(chan struct{}),
		blockchain: blockchain,
	}
}

func (s *Syncer) Register(server *grpc.Server) {
	s.serviceV1 = &serviceV1{logger: hclog.NewNullLogger(), store: s.blockchain, subs: s.blockchain.SubscribeEvents()}
	proto.RegisterV1Server(server, s.serviceV1)
}

func (s *Syncer) Start() {
	go s.serviceV1.start()
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
	go s.watchImpl("", stream)

	currentState, err := clt.GetCurrent(context.Background(), &emptypb.Empty{})
	if err != nil {
		panic(err)
	}

	header, err := s.findCommonAncestor(clt, uint64(currentState.Number))
	if err != nil {
		panic(err)
	}

	fmt.Println("- ancestor header -")
	fmt.Println(header)
}

func (s *Syncer) watchImpl(id string, stream proto.V1_WatchClient) {
	for {
		recv, err := stream.Recv()
		if err != nil {
			panic(err)
		}
		fmt.Println(recv)
	}
}

func (s *Syncer) findCommonAncestor(clt proto.V1Client, targetHeight uint64) (*types.Header, error) {
	h := s.blockchain.Header()

	min := uint64(0) // genesis
	max := h.Number

	if heightNumber := targetHeight; max > heightNumber {
		max = heightNumber
	}

	var header *types.Header
	for min <= max {
		m := uint64(math.Floor(float64(min+max) / 2))

		if m == 0 {
			// our common ancestor is the genesis
			genesis, ok := s.blockchain.GetHeaderByNumber(0)
			if !ok {
				return nil, fmt.Errorf("failed to read local genesis")
			}
			return genesis, nil
		}

		// fmt.Println("- req -")
		// fmt.Println(min, max, m)
		// time.Sleep(1 * time.Second)

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
