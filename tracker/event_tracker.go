package tracker

import (
	"context"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
	boltdbStore "github.com/umbracle/ethgo/tracker/store/boltdb"
)

type eventSubscription interface {
	AddLog(log *ethgo.Log)
}

type EventTracker struct {
	dbPath       string
	rpcEndpoint  string
	contractAddr ethgo.Address
	subscriber   eventSubscription
	logger       hcf.Logger
}

func NewEventTracker(
	dbPath string,
	rpcEndpoint string,
	contractAddr ethgo.Address,
	subscriber eventSubscription,
	logger hcf.Logger,
) *EventTracker {
	return &EventTracker{
		dbPath:       dbPath,
		rpcEndpoint:  rpcEndpoint,
		contractAddr: contractAddr,
		subscriber:   subscriber,
		logger:       logger.Named("event_tracker"),
	}
}

func (e *EventTracker) Start(ctx context.Context) error {
	provider, err := jsonrpc.NewClient(e.rpcEndpoint)
	if err != nil {
		return err
	}

	store, err := boltdbStore.New(e.dbPath)

	if err != nil {
		return err
	}

	e.logger.Info("Start tracking events", "contract address", e.contractAddr)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(10),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: false,
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
				return

			case evnt := <-tt.EventCh:
				if len(evnt.Removed) != 0 {
					panic("this will not happen anymore after tracker v2")
				}

				for _, log := range evnt.Added {
					e.subscriber.AddLog(log)
				}

			case <-tt.DoneCh:
				e.logger.Info("historical sync done")
			}
		}
	}()

	return nil
}
