package bridge

import (
	"github.com/0xPolygon/polygon-edge/bridge/checkpoint"
	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/statesync"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

type Bridge interface {
	Start() error
	Close() error
	SetValidators([]types.Address, uint64)
	StateSync() statesync.StateSync
}

type bridge struct {
	logger     hclog.Logger
	stateSync  statesync.StateSync
	checkpoint checkpoint.Checkpoint
}

func NewBridge(
	logger hclog.Logger,
	network *network.Server,
	signer sam.Signer,
	dataDirURL string,
	config *Config,
) (Bridge, error) {
	bridgeLogger := logger.Named("bridge")

	stateSync, err := statesync.NewStateSync(
		bridgeLogger,
		network,
		signer,
		dataDirURL,
		config.RootChainURL.String(),
		config.RootChainContract,
		config.Confirmations,
	)
	if err != nil {
		return nil, err
	}

	checkpoint, err := checkpoint.NewCheckpoint(
		bridgeLogger,
		network,
		signer,
	)
	if err != nil {
		return nil, err
	}

	return &bridge{
		logger:     bridgeLogger,
		stateSync:  stateSync,
		checkpoint: checkpoint,
	}, nil
}

func (b *bridge) Start() error {
	if err := b.stateSync.Start(); err != nil {
		return err
	}

	if err := b.checkpoint.Start(); err != nil {
		return err
	}

	return nil
}

func (b *bridge) Close() error {
	if err := b.stateSync.Close(); err != nil {
		return err
	}

	if err := b.checkpoint.Close(); err != nil {
		return err
	}

	return nil
}

func (b *bridge) SetValidators(validators []types.Address, threshold uint64) {
	b.stateSync.SetValidators(validators, threshold)
}

func (b *bridge) StateSync() statesync.StateSync {
	return b.stateSync
}
