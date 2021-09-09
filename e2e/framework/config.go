package framework

import (
	"crypto/ecdsa"
	"math/big"
	"path/filepath"

	"github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/crypto"
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
	ReservedPorts           []ReservedPort
	JsonRPCPort             int           // The JSON RPC endpoint port
	GRPCPort                int           // The GRPC endpoint port
	LibP2PPort              int           // The Libp2p endpoint port
	Seal                    bool          // Flag indicating if blocks should be sealed
	RootDir                 string        // The root directory for test environment
	IBFTDirPrefix           string        // The prefix of data directory for IBFT
	IBFTDir                 string        // The name of data directory for IBFT
	PremineAccts            []*SrvAccount // Accounts with existing balances (genesis accounts)
	GenesisValidatorBalance *big.Int      // Genesis balance for the validators
	Consensus               ConsensusType // Consensus Type
	Bootnodes               []string      // Bootnode Addresses
	DevInterval             int           // Dev consensus update interval [s]
	EpochSize               uint64        // The epoch size in blocks for the IBFT layer
	ShowsLog                bool
}

// DataDir returns path of data directory server uses
func (t *TestServerConfig) DataDir() string {
	switch t.Consensus {
	case ConsensusIBFT:
		return filepath.Join(t.RootDir, t.IBFTDir)
	default:
		return t.RootDir
	}
}

// PrivateKey returns a private key in data directory
func (t *TestServerConfig) PrivateKey() (*ecdsa.PrivateKey, error) {
	return crypto.GenerateOrReadPrivateKey(filepath.Join(t.DataDir(), "consensus", ibft.IbftKeyName))
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

// PremineValidatorBalance callback sets the genesis balance of the validator the server manages (in WEI)
func (t *TestServerConfig) PremineValidatorBalance(balance *big.Int) {
	t.GenesisValidatorBalance = balance
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

// SetShowsLog sets flag for logging
func (t *TestServerConfig) SetShowsLog(f bool) {
	t.ShowsLog = f
}

// SetEpochSize sets the epoch size for the consensus layer.
// It controls the rate at which the validator set is updated
func (t *TestServerConfig) SetEpochSize(epochSize uint64) {
	t.EpochSize = epochSize
}
