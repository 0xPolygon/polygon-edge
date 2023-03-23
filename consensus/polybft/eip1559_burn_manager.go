package polybft

import (
	"context"
	"fmt"
	"path"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"

	"github.com/0xPolygon/polygon-edge/types"

	"github.com/0xPolygon/polygon-edge/tracker"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var (
	nativeTokenBurntEventABI = contractsapi.EIP1559Burn.Abi.Events["NativeTokenBurnt"]
)

var _ EIP1559BurnManager = (*eip1559BurnManager)(nil)

type EIP1559BurnManager interface {
	tracker.EventSubscription
	Init() error
	Close()
	PostBlock(req *PostBlockRequest) error
}

// eip1559BurnConfig holds the configuration data of eip1559 burn manager
type eip1559BurnConfig struct {
	eip1559BurnAddr       types.Address
	jsonrpcAddr           string
	startBlock            uint64
	dataDir               string
	numBlockConfirmations uint64
}

type eip1559BurnManager struct {
	logger  hclog.Logger
	config  *eip1559BurnConfig
	closeCh chan struct{}
}

func newEIP1559BurnManager(
	logger hclog.Logger,
	config *eip1559BurnConfig,
) *eip1559BurnManager {
	return &eip1559BurnManager{
		logger:  logger,
		config:  config,
		closeCh: make(chan struct{}),
	}
}

func (m *eip1559BurnManager) Close() {
	close(m.closeCh)
}

func (m *eip1559BurnManager) Init() error {
	if err := m.initTracker(); err != nil {
		return fmt.Errorf("failed to init event tracker: %w", err)
	}

	return nil
}

// initTracker starts a new event tracker to receive NativeTokenBurnt events
func (m *eip1559BurnManager) initTracker() error {
	ctx, cancelFn := context.WithCancel(context.Background())

	evtTracker := tracker.NewEventTracker(
		path.Join(m.config.dataDir, "/eip1559burn.db"),
		m.config.jsonrpcAddr,
		ethgo.Address(m.config.eip1559BurnAddr),
		m,
		m.config.numBlockConfirmations,
		m.config.startBlock,
		m.logger,
	)

	go func() {
		<-m.closeCh
		cancelFn()
	}()

	return evtTracker.Start(ctx)
}

func (m *eip1559BurnManager) AddLog(eventLog *ethgo.Log) {
	if !nativeTokenBurntEventABI.Match(eventLog) {
		return
	}

	m.logger.Info(
		"Add native token burnt event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	// event := &contractsapi.StateSyncedEvent{}
}

func (m *eip1559BurnManager) PostBlock(req *PostBlockRequest) error {
	panic("not implemented yet")
}
