package framework

import (
	"math/big"

	"github.com/0xPolygon/polygon-sdk/types"
)

type ConsensusType int

const (
	ConsensusIBFT ConsensusType = iota
	ConsensusDev
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
	Consensus     ConsensusType // Consensus Type
	Bootnodes     []string      // Bootnode Addresses
	Locals        []string      // Accounts whose transactions are treated as locals
	NoLocals      bool          // Flag to disable price exemptions for locally transactions
	PriceLimit    *uint64       // Minimum gas price limit to enforce for acceptance into the pool
	DevInterval   int           // Dev consensus update interval [s]
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

// SetConsensus callback sets consensus
func (t *TestServerConfig) SetConsensus(c ConsensusType) {
	t.Consensus = c
}

// SetDevInterval sets the update interval for the dev consensus
func (t *TestServerConfig) SetDevInterval(interval int) {
	t.DevInterval = interval
}

// SetIBFTDirPrefix callback sets prefix of IBFT directories
func (t *TestServerConfig) SetIBFTDirPrefix(ibftDirPrefix string) {
	t.IBFTDirPrefix = ibftDirPrefix
}

// SetIBFTDir callback sets the name of data directory for IBFT
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

// SetLocals sets locals
func (t *TestServerConfig) SetLocals(locals []string) {
	t.Locals = locals
}

// SetLocals sets NoLocals flag
func (t *TestServerConfig) SetNoLocals(noLocals bool) {
	t.NoLocals = noLocals
}

// SetLocals sets PriceLimit
func (t *TestServerConfig) SetPriceLimit(priceLimit *uint64) {
	t.PriceLimit = priceLimit
}

// SetShowsLog sets flag for logging
func (t *TestServerConfig) SetShowsLog(f bool) {
	t.ShowsLog = f
}
