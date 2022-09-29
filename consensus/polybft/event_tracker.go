package polybft

import (
	"context"
	"path/filepath"

	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/tracker"
	boltdbStore "github.com/umbracle/ethgo/tracker/store/boltdb"
)

type eventSubscription interface {
	AddLog(log *ethgo.Log)
}

type eventTracker struct {
	dataDir    string
	config     *params.PolyBFTConfig
	subscriber eventSubscription
	logger     hcf.Logger
}

func (e *eventTracker) start() error {
	provider, err := jsonrpc.NewClient(e.config.Bridge.JsonRPCEndpoint)
	if err != nil {
		return err
	}
	store, err := boltdbStore.New(filepath.Join(e.dataDir, "/deposit.db"))
	if err != nil {
		return err
	}

	e.logger.Info("Start tracking events", "bridge", e.config.Bridge.BridgeAddr)

	tt, err := tracker.NewTracker(provider.Eth(),
		tracker.WithBatchSize(10),
		tracker.WithStore(store),
		tracker.WithFilter(&tracker.FilterConfig{
			Async: false,
			Address: []ethgo.Address{
				ethgo.Address(e.config.Bridge.BridgeAddr),
			},
		}),
	)
	if err != nil {
		return err
	}

	go func() {
		go func() {
			if err := tt.Sync(context.Background()); err != nil {
				e.logger.Error("Event tracker", "failed to sync", err)
			}
		}()

		go func() {
			for {
				select {
				case evnt := <-tt.EventCh:
					if len(evnt.Removed) != 0 {
						panic("this will not happen anymore after tracker v2")
					}
					for _, log := range evnt.Added {
						e.subscriber.AddLog(log)
					}
				case <-tt.DoneCh:
					e.logger.Info("Historical sync done")
				}
			}
		}()
	}()

	return nil
}
