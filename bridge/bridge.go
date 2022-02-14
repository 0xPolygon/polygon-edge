package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/bridge/sam"
	"github.com/0xPolygon/polygon-edge/bridge/tracker"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/go-web3"
)

type Bridge interface {
	Start() error
	Close() error
	SetValidators([]types.Address, uint64)
}

type bridge struct {
	logger hclog.Logger

	sampool sam.Manager
	tracker *tracker.Tracker

	closeCh chan struct{}
}

func NewBridge(
	logger hclog.Logger,
	network *network.Server,
	signer sam.Signer,
	recoverer sam.SignatureRecoverer,
	confirmations uint64,
) (Bridge, error) {
	fmt.Printf("NewBridge confirmations %d\n", confirmations)

	bridgeLogger := logger.Named("bridge")

	tracker, err := tracker.NewEventTracker(bridgeLogger, confirmations)
	if err != nil {
		return nil, err
	}

	return &bridge{
		logger:  bridgeLogger,
		sampool: sam.NewManager(bridgeLogger, signer, recoverer, network, nil, 0),
		tracker: tracker,
		closeCh: make(chan struct{}),
	}, nil
}

func (b *bridge) Start() error {
	if err := b.sampool.Start(); err != nil {
		return err
	}
	if err := b.tracker.Start(); err != nil {
		return err
	}

	eventCh := b.tracker.GetEventChannel()
	go b.processEvents(eventCh)

	return nil
}

func (b *bridge) Close() error {
	close(b.closeCh)

	if err := b.sampool.Close(); err != nil {
		return err
	}
	if err := b.tracker.Stop(); err != nil {
		return err
	}

	return nil
}

func (b *bridge) SetValidators(validators []types.Address, threshold uint64) {
	b.sampool.UpdateValidatorSet(validators, threshold)
}

func (b *bridge) processEvents(eventCh <-chan []byte) {
	for {
		select {
		case <-b.closeCh:
			return
		case data := <-eventCh:
			if err := b.processEthEvent(data); err != nil {
				b.logger.Error("failed to process event", "err", err)
			}
		}
	}
}

func (b *bridge) processEthEvent(data []byte) error {
	// workaround
	var log web3.Log
	if err := json.Unmarshal(data, &log); err != nil {
		return err
	}
	if !tracker.StateSyncedEvent.Match(&log) {
		return nil
	}
	fmt.Printf("StateSyncedEvent %+v\n", log)

	record, err := tracker.StateSyncedEvent.ParseLog(&log)
	if err != nil {
		return err
	}

	var id *big.Int
	var addr web3.Address
	var ok bool

	id, ok = record["id"].(*big.Int)
	if !ok {
		return errors.New("failed to parse ID")
	}

	addr, ok = record["contractAddress"].(web3.Address)
	if !ok {
		return errors.New("failed to parse contractAddress")
	}

	data, ok = record["data"].([]uint8)
	if !ok {
		return errors.New("failed to parse data")
	}

	fmt.Printf("id=%+v, addr=%+v\n", id, addr)
	fmt.Printf("data=%+v\n", data)

	err = b.sampool.AddMessage(&sam.Message{
		ID:   id.Uint64(),
		Body: data,
	})

	if err != nil {
		return err
	}

	return nil
}
