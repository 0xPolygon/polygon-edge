package tracker

import (
	"context"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
)

type eventSubscription interface {
	AddLog(log *ethgo.Log)
}

type EventTracker struct {
	dbPath        string
	rpcEndpoint   string
	contractAddr  ethgo.Address
	subscriber    eventSubscription
	logger        hcf.Logger
	finalityDepth uint64 // minimal number of child blocks required for the parent block to be considered final
}

func NewEventTracker(
	dbPath string,
	rpcEndpoint string,
	contractAddr ethgo.Address,
	subscriber eventSubscription,
	finalityDepth uint64,
	logger hcf.Logger,
) *EventTracker {
	return &EventTracker{
		dbPath:        dbPath,
		rpcEndpoint:   rpcEndpoint,
		contractAddr:  contractAddr,
		subscriber:    subscriber,
		finalityDepth: finalityDepth,
		logger:        logger.Named("event_tracker"),
	}
}

func (e *EventTracker) Start(ctx context.Context) error {
	provider, err := jsonrpc.NewClient(e.rpcEndpoint)
	if err != nil {
		return err
	}

	notifierCh := make(chan []*ethgo.Log)

	store, err := NewEventTrackerStore(e.dbPath, e.finalityDepth, notifierCh, e.logger)
	if err != nil {
		return err
	}

	e.logger.Info("Start tracking events", "block finality depth", e.finalityDepth, "contract address", e.contractAddr)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(10),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: true,
			Address: []ethgo.Address{
				e.contractAddr,
			},
		}),
	)
	if err != nil {
		return err
	}

	go func() {
		if err := tt.Sync(ctx); err != nil {
			e.logger.Error("failed to sync", "error", err)
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				if err := store.Close(); err != nil {
					e.logger.Info("error while closing store", "err", err)
				}

				close(notifierCh) // close notifier channel after everything is finished

				return

			case evnt := <-notifierCh:
				for _, log := range evnt {
					e.subscriber.AddLog(log)
				}

			case <-tt.DoneCh:
				e.logger.Info("historical sync done")
			}
		}
	}()

	return nil
}
