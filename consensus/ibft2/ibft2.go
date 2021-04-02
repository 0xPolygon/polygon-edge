package ibft2

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/minimal/blockchain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/txpool"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

type Ibft2 struct {
	logger hclog.Logger
	config *consensus.Config

	state uint64

	blockchain   *blockchain.Blockchain
	executor     *state.Executor
	closeCh      chan struct{}
	validatorKey *ecdsa.PrivateKey

	network *network.Server

	transportFactory transportFactory
	transport        transport
}

func Factory(ctx context.Context, config *consensus.Config, txpool *txpool.TxPool, blockchain *blockchain.Blockchain, executor *state.Executor, privateKey *ecdsa.PrivateKey, logger hclog.Logger) (consensus.Consensus, error) {
	p := &Ibft2{
		logger:           logger.Named("ibft2"),
		config:           config,
		blockchain:       blockchain,
		executor:         executor,
		closeCh:          make(chan struct{}),
		transportFactory: grpcTransportFactory,
	}
	return p, nil
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

func (i *Ibft2) start() {
	fmt.Println("- run -")

	for {
		select {
		case <-i.closeCh:
			return
		default:
		}

		switch i.getState() {
		case AcceptState:
			i.runAcceptState()

		//case RoundChangeState:
		//	i.runStateChange()

		case PreprepareState:
			i.runPreprepare()

		case PrepareState:
			i.runPrepare()

		case CommitState:
			i.runCommit()
		}
	}
}

func (i *Ibft2) runAcceptState() {

}

func (i *Ibft2) runStateChange() {
	// TODO
}

func (i *Ibft2) runPreprepare() {
	//timer := randomTimeout(1 * time.Second)

	for i.getState() == CommitState {
		/*
			select {
			case msg := <-b.preprepareCh:
				fmt.Println("-- msg --")
				fmt.Println(msg)

			case <-timer:
				b.setState(RoundChangeState)
			}
		*/
	}
}

func (i *Ibft2) runPrepare() {

}

func (i *Ibft2) runCommit() {
	//timer := randomTimeout(1 * time.Second)

	for i.getState() == CommitState {
		/*
			select {
			case vote := <-b.commitCh:
				// accept vote
				if !b.roundState.verify(vote) {

				}
			case <-timer:
				b.setState(RoundChangeState)
			}
		*/
	}
}

func (i *Ibft2) getState() IbftState {
	stateAddr := (*uint64)(&i.state)
	return IbftState(atomic.LoadUint64(stateAddr))
}

func (i *Ibft2) setState(s IbftState) {
	i.logger.Debug("state change", "new", s)

	stateAddr := (*uint64)(&i.state)
	atomic.StoreUint64(stateAddr, uint64(s))
}

func randomTimeout(minVal time.Duration) <-chan time.Time {
	if minVal == 0 {
		return nil
	}
	extra := (time.Duration(rand.Int63()) % minVal)
	return time.After(minVal + extra)
}

func (i *Ibft2) StartSeal() {
	// generate a validator private key
	validatorKey, err := crypto.ReadPrivKey(filepath.Join(i.config.Path, "validator.key"))
	if err != nil {
		panic(err)
	}
	i.validatorKey = validatorKey

	// start the transport protocol
	transport, err := i.transportFactory(i)
	if err != nil {
		panic(err)
	}
	i.transport = transport

	go i.run()
}

func (i *Ibft2) run() {

}

func (i *Ibft2) VerifyHeader(parent, header *types.Header) error {
	return nil
}

func (i *Ibft2) Close() error {
	close(i.closeCh)
	return nil
}

// TODO REMOVE
func (i *Ibft2) Prepare(header *types.Header) error {
	return nil
}

// TODO REMOVE
func (i *Ibft2) Seal(block *types.Block, ctx context.Context) (*types.Block, error) {
	return nil, nil
}
