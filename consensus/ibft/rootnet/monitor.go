package rootnet

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/rootchain/samuel"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
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
	config *rootchain.Config,
	signer signer,
) (Monitor, error) {
	//	init tracker
	//	init sampool
	//	init samuel

	return &monitor{}, nil
}

type monitor struct {
	samuel samuel.SAMUEL
}

func (m *monitor) PeekTransaction() *types.Transaction {
	return m.samuel.GetReadyTransaction()
}

func (m *monitor) PopTransaction() {
	m.samuel.PopReadyTransaction()
}

func (m *monitor) SaveProgress(block *types.Block) {
	// todo: milos
	//m.samuel.SaveProgress()
}
