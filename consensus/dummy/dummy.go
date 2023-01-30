package dummy

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/helper/progress"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/syncer"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	hclog "github.com/hashicorp/go-hclog"
	"time"
)

type Dummy struct {
	logger     hclog.Logger
	notifyCh   chan struct{}
	closeCh    chan struct{}
	txpool     *txpool.TxPool
	blockchain *blockchain.Blockchain
	executor   *state.Executor
	syncer     syncer.Syncer
}

func Factory(params *consensus.Params) (consensus.Consensus, error) {
	logger := params.Logger.Named("dummy")

	d := &Dummy{
		logger:     logger,
		notifyCh:   make(chan struct{}),
		closeCh:    make(chan struct{}),
		blockchain: params.Blockchain,
		executor:   params.Executor,
		txpool:     params.TxPool,

		syncer: syncer.NewSyncer(
			params.Logger,
			params.Network,
			params.Blockchain,
			time.Duration(params.BlockTime)*3*time.Second,
		),
	}

	return d, nil
}

// Initialize initializes the consensus
func (d *Dummy) Initialize() error {
	d.txpool.SetSealing(true)

	return nil
}

func (d *Dummy) Start() error {

	// Start the syncer
	if err := d.syncer.Start(); err != nil {
		return err
	}

	// Start syncing blocks from other peers
	go d.startSyncing()

	go d.run()

	return nil
}

func (d *Dummy) VerifyHeader(header *types.Header) error {
	// All blocks are valid
	fmt.Printf("Verifying %d -> %s", header.Number, header.Hash)
	return nil
}

func (d *Dummy) ProcessHeaders(headers []*types.Header) error {
	return nil
}

func (d *Dummy) GetBlockCreator(header *types.Header) (types.Address, error) {
	return types.BytesToAddress(header.Miner), nil
}

// PreCommitState a hook to be called before finalizing state transition on inserting block
func (d *Dummy) PreCommitState(_header *types.Header, _txn *state.Transition) error {
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

func (d *Dummy) startSyncing() {
	callInsertBlockHook := func(block *types.Block) bool {
		/*
			if err := d.currentHooks.PostInsertBlock(block); err != nil {
				d.logger.Error("failed to call PostInsertBlock", "height", block.Header.Number, "error", err)
			}

			if err := d.updateCurrentModules(block.Number() + 1); err != nil {
				d.logger.Error("failed to update sub modules", "height", block.Number()+1, "err", err)
			}
		*/
		d.logger.Info("Block %d", block.Number())

		d.txpool.ResetWithHeaders(block.Header)

		return false
	}

	if err := d.syncer.Sync(
		callInsertBlockHook,
	); err != nil {
		d.logger.Error("watch sync failed", "err", err)
	}
}
