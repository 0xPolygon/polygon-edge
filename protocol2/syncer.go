package protocol2

import (
	"context"
	"fmt"
	"math/big"
	"sync"

	"github.com/0xPolygon/minimal/protocol2/proto"
	v1 "github.com/0xPolygon/minimal/protocol2/proto"
	"github.com/0xPolygon/minimal/types"
	"google.golang.org/grpc"
)

// Event to be removed later
type Event struct {
	OldChain []*types.Header
	NewChain []*types.Header
}

type subscription interface {
	GetEvent() *Event
	Close()
}

// blockchain is the interface required by the syncer to connect to the blockchain
type blockchain interface {
	SubscribeEvents() subscription
}

// Syncer is a sync protocol
type Syncer struct {
	headWatcher *headWatcher

	peersLock sync.Mutex
	peers     map[string]*peer

	// notify a change in the target peer
	updateCh chan *peer

	stopCh chan struct{}
}

type peer struct {
	client     *proto.V1Client
	difficulty *big.Int
	updateCh   chan *proto.Component
}

func NewSyncer() *Syncer {
	return &Syncer{
		headWatcher: &headWatcher{},
		peers:       map[string]*peer{},
		stopCh:      make(chan struct{}),
	}
}

func (s *Syncer) Start() {
	go s.run()
}

func (s *Syncer) handleUser(conn *grpc.ClientConn) {
	go func() {
		clt := proto.NewV1Client(conn)

		recv, err := clt.WatchBlocks(context.Background(), &v1.Empty{})
		if err != nil {
			panic(err)
		}
		for {
			msg, err := recv.Recv()
			if err != nil {
				panic(err)
			}
			fmt.Println("- msg -")
			fmt.Println(msg)
		}
	}()
}

func (s *Syncer) run() {
	var cancelFn context.CancelFunc
	var ctx context.Context

	for {
		select {
		case target := <-s.updateCh:
			if cancelFn != nil {
				cancelFn()
			}
			ctx, cancelFn = context.WithCancel(context.Background())
			go func() {
				s.syncTarget(ctx, target)
			}()

		case <-s.stopCh:
			if cancelFn != nil {
				cancelFn()
			}
		}
	}
}

func (s *Syncer) syncTarget(ctx context.Context, target *peer) error {
	var latestKnownBlock, latestSyncedBlock *types.Header
	var err error

	// batch sync
	for {
		// find common ancenstor
		latestKnownBlock, latestSyncedBlock, err = s.findAncestor(ctx, nil)
		if err != nil {
			return err
		}

		// check if we are at least n blocks away
		if latestKnownBlock.Number-20 < latestSyncedBlock.Number {
			break
		}

		// sync with ancestor
		if err := s.batchSyncTo(ctx, nil); err != nil {
			return err
		}
	}

	// tip sync
	for i := latestSyncedBlock.Number; i < latestKnownBlock.Number; i++ {

	}

	// wait for updates
	return nil
}

func (s *Syncer) findAncestor(ctx context.Context, target *proto.V1Client) (*types.Header, *types.Header, error) {
	return nil, nil, nil
}

func (s *Syncer) batchSyncTo(ctx context.Context, target *proto.V1Client) error {
	return nil
}
