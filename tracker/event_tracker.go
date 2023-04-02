package tracker

import (
	"context"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/blocktracker"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
)

type eventSubscription interface {
	AddLog(log *ethgo.Log)
}

type EventTracker struct {
	dbPath                string
	rpcEndpoint           string
	contractAddr          ethgo.Address
	startBlock            uint64
	subscriber            eventSubscription
	logger                hcf.Logger
	numBlockConfirmations uint64 // minimal number of child blocks required for the parent block to be considered final
}

func NewEventTracker(
	dbPath string,
	rpcEndpoint string,
	contractAddr ethgo.Address,
	subscriber eventSubscription,
	numBlockConfirmations uint64,
	startBlock uint64,
	logger hcf.Logger,
) *EventTracker {
	return &EventTracker{
		dbPath:                dbPath,
		rpcEndpoint:           rpcEndpoint,
		contractAddr:          contractAddr,
		subscriber:            subscriber,
		numBlockConfirmations: numBlockConfirmations,
		startBlock:            startBlock,
		logger:                logger.Named("event_tracker"),
	}
}

func (e *EventTracker) Start(ctx context.Context) error {
	e.logger.Info("Start tracking events",
		"contract", e.contractAddr,
		"JSON RPC address", e.rpcEndpoint,
		"num block confirmations", e.numBlockConfirmations,
		"start block", e.startBlock)

	provider, err := jsonrpc.NewClient(e.rpcEndpoint)
	if err != nil {
		return err
	}

	store, err := NewEventTrackerStore(e.dbPath, e.numBlockConfirmations, e.subscriber, e.logger)
	if err != nil {
		return err
	}

	blockMaxBacklog := e.numBlockConfirmations*2 + 1
	blockTracker := blocktracker.NewBlockTracker(provider.Eth(), blocktracker.WithBlockMaxBacklog(blockMaxBacklog))

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(10),
		tracker.WithBlockTracker(blockTracker),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: true,
			Address: []ethgo.Address{
				e.contractAddr,
			},
			Start: e.startBlock,
		}),
	)
	if err != nil {
		return err
	}

	go func() {
		if err := blockTracker.Init(); err != nil {
			e.logger.Error("failed to init blocktracker", "error", err)

			return
		}

		if err := blockTracker.Start(); err != nil {
			e.logger.Error("failed to start blocktracker", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		blockTracker.Close()
		store.Close()
	}()

	go func() {
		if err := tt.Sync(ctx); err != nil {
			e.logger.Error("failed to sync", "error", err)
		}
	}()

	return nil
}
