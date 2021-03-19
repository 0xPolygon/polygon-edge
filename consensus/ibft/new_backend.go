package ibft

import (
	"context"
	"sync/atomic"

	"github.com/0xPolygon/minimal/consensus/ibft/aux"
	"github.com/0xPolygon/minimal/types"
	"google.golang.org/grpc"
)

type IbftState uint32

const (
	// Follower is the initial state of a Raft node.
	Follower IbftState = iota

	// Candidate is one of the valid states of a Raft node.
	Candidate

	// Leader is one of the valid states of a Raft node.
	Leader

	// Shutdown is the terminal state of a Raft node.
	Shutdown
)

// Backend2 is the IBFT consensus implementation
type Backend2 struct {
	state     uint64 // keep this at the top always
	grpc      grpc.Server
	snapState *snapshotState
	store     aux.BlockchainInterface
	readyCh   chan struct{}
	closeCh   chan struct{}
}

func (b *Backend2) run() {
	for {
		select {
		case <-b.closeCh:
			return
		default:
		}

		switch b.getState() {
		case Follower:
			b.runFollower()
		case Leader:
			b.runLeader()
		}
	}
}

func (b *Backend2) runFollower() {

}

func (b *Backend2) runLeader() {

}

func (b *Backend2) getState() IbftState {
	stateAddr := (*uint64)(&b.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (b *Backend2) setState(s IbftState) {
	stateAddr := (*uint64)(&b.state)
	atomic.StoreUint64(stateAddr, uint64(s))
}

func (b *Backend2) VerifyHeader(parent, header *types.Header, uncle, seal bool) error {
	// TODO: Verify all the easy things

	b.snapState.verifyHeader(header)
	return nil
}

func (b *Backend2) Prepare(header *types.Header) error {
	return nil
}

func (b *Backend2) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	return nil, nil
}

func (b *Backend2) Close() error {
	return nil
}
