package tracker

import (
	"context"
	"fmt"

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

// Start will initialize Block Tracker and Tracker instances and begin tracking events.
// The Tracker will perform a concurrent sync operation which may take some time.
// The error channel should be used to handle errors that occur from the concurrent sync operation.
func (e *EventTracker) Start(ctx context.Context) (error, chan error) {
	e.logger.Info("Start tracking events",
		"contract", e.contractAddr,
		"JSON RPC address", e.rpcEndpoint,
		"num block confirmations", e.numBlockConfirmations,
		"start block", e.startBlock)

	provider, err := jsonrpc.NewClient(e.rpcEndpoint)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %w", err), nil
	}

	// Block Tracker
	blockMaxBacklog := e.numBlockConfirmations*2 + 1
	blockTracker := blocktracker.NewBlockTracker(provider.Eth(), blocktracker.WithBlockMaxBacklog(blockMaxBacklog))
	if err := blockTracker.Init(); err != nil {
		return fmt.Errorf("failed to init blocktracker: %w", err), nil
	}
	if err := blockTracker.Start(); err != nil {
		return fmt.Errorf("failed to start blocktracker: %w", err), nil
	}

	// Tracker
	store, err := NewEventTrackerStore(e.dbPath, e.numBlockConfirmations, e.subscriber, e.logger)
	if err != nil {
		return fmt.Errorf("failed to create tracker store: %w", err), nil
	}
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
		return err, nil
	}

	// Clean up block tracker and store when required
	go func() {
		<-ctx.Done()
		blockTracker.Close()
		store.Close()
	}()

	// Run sync concurrently and return error channel
	syncErrCh := make(chan error)
	go func() {
		syncErrCh <- tt.Sync(ctx)
		close(syncErrCh)
	}()
	return nil, syncErrCh
}
