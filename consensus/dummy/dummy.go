package dummy

import (
	"github.com/dogechain-lab/jury/blockchain"
	"github.com/dogechain-lab/jury/consensus"
	"github.com/dogechain-lab/jury/helper/progress"
	"github.com/dogechain-lab/jury/state"
	"github.com/dogechain-lab/jury/txpool"
	"github.com/dogechain-lab/jury/types"
	"github.com/hashicorp/go-hclog"
)

type Dummy struct {
	sealing    bool
	logger     hclog.Logger
	notifyCh   chan struct{}
	closeCh    chan struct{}
	txpool     *txpool.TxPool
	blockchain *blockchain.Blockchain
	executor   *state.Executor
}

func Factory(params *consensus.ConsensusParams) (consensus.Consensus, error) {
	logger := params.Logger.Named("dummy")

	d := &Dummy{
		sealing:    params.Seal,
		logger:     logger,
		notifyCh:   make(chan struct{}),
		closeCh:    make(chan struct{}),
		blockchain: params.Blockchain,
		executor:   params.Executor,
		txpool:     params.Txpool,
	}

	return d, nil
}

// Initialize initializes the consensus
func (d *Dummy) Initialize() error {
	return nil
}

func (d *Dummy) Start() error {
	go d.run()

	return nil
}

func (d *Dummy) VerifyHeader(parent *types.Header, header *types.Header) error {
	// All blocks are valid
	return nil
}

func (d *Dummy) GetBlockCreator(header *types.Header) (types.Address, error) {
	return header.Miner, nil
}

// PreStateCommit a hook to be called before finalizing state transition on inserting block
func (d *Dummy) PreStateCommit(_header *types.Header, _txn *state.Transition) error {
	return nil
}

func (d *Dummy) GetSyncProgression() *progress.Progression {
	return nil
}

func (d *Dummy) Close() error {
	close(d.closeCh)

	return nil
}

func (d *Dummy) run() {
	d.logger.Info("started")
	// do nothing
	<-d.closeCh
}
