package rootnet

import (
	"github.com/0xPolygon/polygon-edge/blockchain/storage/leveldb"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/sampool"
	"github.com/0xPolygon/polygon-edge/rootchain/samuel"
	"github.com/0xPolygon/polygon-edge/rootchain/tracker"
	"github.com/0xPolygon/polygon-edge/rootchain/transport"
	"github.com/0xPolygon/polygon-edge/types"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
)

var (
	NilMonitor = nilMonitor{}
)

// signer defines the signer interface used for
// generating signatures
type signer interface {
	// Sign signs the specified data,
	// and returns the signature and the block number at which
	// the signature was generated
	Sign([]byte) ([]byte, uint64, error)

	// VerifySignature verifies the signature for the passed in
	// raw data, and at the specified block number
	VerifySignature([]byte, []byte, uint64) error

	// Quorum returns the number of quorum validators
	// for the given block number
	Quorum(uint64) uint64
}

type Monitor interface {
	PeekTransaction() *types.Transaction
	PopTransaction()
	SaveProgress(block *types.Block)
}

func NewMonitor(
	logger hclog.Logger,
	config rootchain.EventConfig,
	signer signer,
	network *network.Server,
	rootDir string,
) (Monitor, error) {
	//	init tracker
	tracker, err := tracker.NewEventTracker(logger, config)
	if err != nil {
		return nil, err
	}

	//	init sampool
	pool := sampool.New(logger) // todo

	storage, err := leveldb.NewLevelDBStorage(
		filepath.Join(rootDir, "rootnet"),
		logger,
	)
	if err != nil {
		return nil, err
	}

	//	init samuel
	samuel := samuel.NewSamuel(
		config,
		logger,
		tracker,
		pool,
		signer,
		storage,
		transport.NewLibp2pGossipTransport(
			logger,
			network,
		),
	)

	return &monitor{samuel}, nil
}

type monitor struct {
	samuel *samuel.SAMUEL
}

func (m *monitor) PeekTransaction() *types.Transaction {
	return m.samuel.GetReadyTransaction()
}

func (m *monitor) PopTransaction() {
	m.samuel.PopReadyTransaction()
}

func (m *monitor) SaveProgress(block *types.Block) {
	stateTxs := block.ExtractStateTransactions()
	if len(stateTxs) == 0 {
		return
	}

	// todo: milos
	//m.samuel.SaveProgress()
}

type nilMonitor struct{}

func (m nilMonitor) PeekTransaction() *types.Transaction { return nil }

func (m nilMonitor) PopTransaction() {}

func (m nilMonitor) SaveProgress(block *types.Block) {}
