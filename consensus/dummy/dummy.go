package dummy

import (
	"github.com/0xPolygon/polygon-sdk/blockchain"
	"github.com/0xPolygon/polygon-sdk/consensus"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/txpool"
	"github.com/0xPolygon/polygon-sdk/types"
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

	params.Txpool.NotifyCh = d.notifyCh

	return d, nil
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

func (d *Dummy) Close() error {
	close(d.closeCh)
	return nil
}

func (d *Dummy) run() {
	d.logger.Info("started")
	// do nothing
	<-d.closeCh
}
