package ibft

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft/aux"
	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
)

type ProposerPolicy uint64

const (
	RoundRobin ProposerPolicy = iota
	Sticky
)

type Config struct {
	RequestTimeout uint64         // The timeout for each Istanbul round in milliseconds.
	BlockPeriod    uint64         // Default minimum difference between two consecutive block's timestamps in second
	ProposerPolicy ProposerPolicy // The policy for proposer selection
	Epoch          uint64         // The number of blocks after which to checkpoint and reset the pending votes
}

var DefaultConfig = &Config{
	RequestTimeout: 10000,
	BlockPeriod:    1,
	ProposerPolicy: RoundRobin,
	Epoch:          30000,
}

// Factory is the factory method to create an Ethash consensus
func Factory(ctx context.Context, blockchain *blockchain.Blockchain, config *consensus.Config, privateKey *ecdsa.PrivateKey, logger hclog.Logger) (consensus.Consensus, error) {
	/*
		var pathStr string
		path, ok := config.Config["path"]
		if ok {
			pathStr, ok = path.(string)
			if !ok {
				return nil, fmt.Errorf("could not convert path to string")
			}
		}
	*/

	bb := &Backend2{
		logger:  logger.Named("ibft"),
		config:  DefaultConfig, // TODO: Decode from consensusConfig
		store:   blockchain,
		address: crypto.PubKeyToAddress(&privateKey.PublicKey),
	}

	if err := bb.Setup(); err != nil {
		return nil, err
	}
	go bb.run()
	return bb, nil
}

func (b *Backend2) Setup() error {
	b.snapState = &snapshotState{
		store:   b.store,
		config:  b.config,
		readyCh: make(chan struct{}),
	}
	if err := b.snapState.setup(); err != nil {
		return err
	}

	// start creating block proposals
	b.setState(AcceptState)

	return nil
}

type IbftState uint32

const (
	AcceptState IbftState = iota
	RoundChangeState
	PreprepareState
	PrepareState
	CommitState
)

func (i IbftState) String() string {
	switch i {
	case AcceptState:
		return "AcceptState"

	case RoundChangeState:
		return "RoundChangeState"

	case PreprepareState:
		return "PreprepareState"

	case PrepareState:
		return "PrepareState"

	case CommitState:
		return "CommitState"
	}
	panic(fmt.Sprintf("BUG: Ibft state not found %d", i))
}

type transport interface {
	broadcast(msg *proto.MessageReq)
}

// Backend2 is the IBFT consensus implementation
type Backend2 struct {
	logger hclog.Logger

	state      uint64 // keep this at the top always
	grpc       grpc.Server
	snapState  *snapshotState
	store      aux.BlockchainInterface
	readyCh    chan struct{}
	closeCh    chan struct{}
	config     *Config
	txPool     *txpool.TxPool2
	transport  transport
	roundState *roundState
	address    types.Address

	// protocol messages
	commitCh      chan *proto.Subject
	preprepareCh  chan *proto.Subject
	prepareCh     chan *proto.Subject
	stateChangeCh chan *proto.Subject
}

func (b *Backend2) run() {
	fmt.Println("- run -")

	for {
		select {
		case <-b.closeCh:
			return
		default:
		}

		switch b.getState() {
		case AcceptState:
			b.runAcceptState()

		//case RoundChangeState:
		//	b.runStateChange()

		case PreprepareState:
			b.runPreprepare()

		case PrepareState:
			b.runPrepare()

		case CommitState:
			b.runCommit()
		}
	}
}

func (b *Backend2) runAcceptState() {
	// get latest proposal
	lastProposal, ok := b.store.GetHeaderByHash(b.store.Header().Hash)
	if !ok {
		panic("bad")
	}

	snap, err := b.snapState.getSnapshot(int(lastProposal.Number - 1))
	if err != nil {
		panic(err)
	}

	// we need to decide from the validators list which is the
	// next block proposer
	var lastProposer types.Address
	if lastProposal.Number != 0 {
		if lastProposer, err = ecrecover(lastProposal); err != nil {
			panic(err)
		}
	}

	proposer := snap.NextValidator(lastProposer)

	fmt.Println("-- addr --")
	fmt.Println(snap.Set)
	fmt.Println(b.address)
	fmt.Println(proposer)

	if proposer == b.address {
		// we are the proposers
		proposal := &types.Header{
			ParentHash: lastProposal.Hash,
			Number:     lastProposal.Number + 1,
		}

		b.logger.Debug("proposing block", "number", proposal.Number)

		// broadcast the preprepare message
		b.transport.broadcast(&proto.MessageReq{
			Message: &proto.MessageReq_Preprepare{
				Preprepare: &proto.Preprepare{
					Proposal: &proto.Proposal{
						Block: &any.Any{
							Value: proposal.MarshalRLP(),
						},
					},
				},
			},
		})
	} else {
		// wait in preprepare state
		b.setState(PreprepareState)
	}
}

func (b *Backend2) runStateChange() {
	// TODO
}

func (b *Backend2) runPreprepare() {
	timer := randomTimeout(1 * time.Second)

	for b.getState() == CommitState {
		select {
		case msg := <-b.preprepareCh:
			fmt.Println("-- msg --")
			fmt.Println(msg)

		case <-timer:
			b.setState(RoundChangeState)
		}
	}
}

func (b *Backend2) runPrepare() {

}

func (b *Backend2) runCommit() {
	timer := randomTimeout(1 * time.Second)

	for b.getState() == CommitState {
		select {
		case vote := <-b.commitCh:
			// accept vote
			if !b.roundState.verify(vote) {

			}
		case <-timer:
			b.setState(RoundChangeState)
		}
	}
}

func randomTimeout(minVal time.Duration) <-chan time.Time {
	if minVal == 0 {
		return nil
	}
	extra := (time.Duration(rand.Int63()) % minVal)
	return time.After(minVal + extra)
}

func (b *Backend2) getState() IbftState {
	stateAddr := (*uint64)(&b.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (b *Backend2) setState(s IbftState) {
	b.logger.Debug("state change", "new", s)

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

// holds the sequence and round
type roundState struct {
	round    uint64
	sequence uint64
	digest   string
}

// verify returns whether the Subject matches the current roundState
func (r *roundState) verify(s *proto.Subject) bool {
	if r.round != s.View.Round {
		return false
	}
	if r.sequence != s.View.Sequence {
		return false
	}
	if r.digest != s.Digest {
		return false
	}
	return true
}

// cmp compares a view with respect to the current roundState
func (r *roundState) cmp(view *proto.View) int {
	if r.round != view.Round {
		return cmpImpl(r.round, view.Round)
	}
	if r.sequence != view.Sequence {
		return cmpImpl(r.sequence, view.Sequence)
	}
	return 0
}

func cmpImpl(a, b uint64) int {
	if a == b {
		return 0
	}
	if a < b {
		return -1
	}
	return 1
}
