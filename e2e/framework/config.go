package framework

import (
	"math/big"

	"github.com/0xPolygon/minimal/types"
)

type ConsensusType int

const (
	ConsensusIBFT ConsensusType = iota
	ConsensusDummy
)

type SrvAccount struct {
	Addr    types.Address
	Balance *big.Int
}

// TestServerConfig for the test server
type TestServerConfig struct {
	ReservedPorts []ReservedPort
	JsonRPCPort   int           // The JSON RPC endpoint port
	GRPCPort      int           // The GRPC endpoint port
	LibP2PPort    int           // The Libp2p endpoint port
	Seal          bool          // Flag indicating if blocks should be sealed
	RootDir       string        // The root directory for test environment
	IBFTDirPrefix string        // The prefix of data directory for IBFT
	IBFTDir       string        // The name of data directory for IBFT
	PremineAccts  []*SrvAccount // Accounts with existing balances (genesis accounts)
	DevMode       bool          // Toggles the dev mode
	Consensus     ConsensusType // Consensus Type
	Bootnodes     []string      // Bootnode Addresses
	ShowsLog      bool
}

// CALLBACKS //
// Premine callback specifies an account with a balance (in WEI)
func (t *TestServerConfig) Premine(addr types.Address, amount *big.Int) {
	if t.PremineAccts == nil {
		t.PremineAccts = []*SrvAccount{}
	}
	t.PremineAccts = append(t.PremineAccts, &SrvAccount{
		Addr:    addr,
		Balance: amount,
	})
}

// SetDev callback toggles the dev mode
func (t *TestServerConfig) SetDev(state bool) {
	t.DevMode = state
}

// SetConsensus callback sets consensus
func (t *TestServerConfig) SetConsensus(c ConsensusType) {
	t.Consensus = c
}

// SetIBFTDirPrefix callback sets prefix of IBFT directories
func (t *TestServerConfig) SetIBFTDirPrefix(ibftDirPrefix string) {
	t.IBFTDirPrefix = ibftDirPrefix
}

// SetIBFTDirPrefix callback sets prefix of IBFT directories
func (t *TestServerConfig) SetIBFTDir(ibftDir string) {
	t.IBFTDir = ibftDir
}

// SetSeal callback toggles the seal mode
func (t *TestServerConfig) SetSeal(state bool) {
	t.Seal = state
}

// SetBootnodes sets bootnodes
func (t *TestServerConfig) SetBootnodes(bootnodes []string) {
	t.Bootnodes = bootnodes
}

// SetBootnodes sets bootnodes
func (t *TestServerConfig) SetShowsLog(f bool) {
	t.ShowsLog = f
}
