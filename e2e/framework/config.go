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
	JsonRPCPort             int             // The JSON RPC endpoint port
	GRPCPort                int             // The GRPC endpoint port
	LibP2PPort              int             // The Libp2p endpoint port
	Seal                    bool            // Flag indicating if blocks should be sealed
	RootDir                 string          // The root directory for test environment
	IBFTDirPrefix           string          // The prefix of data directory for IBFT
	IBFTDir                 string          // The name of data directory for IBFT
	PremineAccts            []*SrvAccount   // Accounts with existing balances (genesis accounts)
	GenesisValidatorBalance *big.Int        // Genesis balance for the validators
	DevStakers              []types.Address // List of initial staking addresses for the staking SC with dev consensus
	Consensus               ConsensusType   // Consensus MechanismType
	Bootnodes               []string        // Bootnode Addresses
	Locals                  []string        // Accounts whose transactions are treated as locals
	NoLocals                bool            // Flag to disable price exemptions for locally transactions
	PriceLimit              *uint64         // Minimum gas price limit to enforce for acceptance into the pool
	DevInterval             int             // Dev consensus update interval [s]
	EpochSize               uint64          // The epoch size in blocks for the IBFT layer
	BlockGasLimit           uint64          // Block gas limit
	BlockGasTarget          uint64          // Gas target for new blocks
	ShowsLog                bool
	IsPos                   bool // Specifies the mechanism used for IBFT (PoA / PoS)
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

// SetBlockGasTarget sets the gas target for the test server
func (t *TestServerConfig) SetBlockGasTarget(target uint64) {
	t.BlockGasTarget = target
}

// SetConsensus callback sets consensus
func (t *TestServerConfig) SetConsensus(c ConsensusType) {
	t.Consensus = c
}

// SetDevInterval sets the update interval for the dev consensus
func (t *TestServerConfig) SetDevInterval(interval int) {
	t.DevInterval = interval
}

// SetDevStakingAddresses sets the Staking smart contract staker addresses for the dev mode.
// These addresses should be passed into the `ibft-validator` flag in genesis generation.
// Since invoking the dev consensus will not generate the ibft base folders, this is the only way
// to signalize to the genesis creation process who the validators are
func (t *TestServerConfig) SetDevStakingAddresses(stakingAddresses []types.Address) {
	t.DevStakers = stakingAddresses
}

// SetIBFTPoS sets the flag indicating the IBFT mechanism
func (t *TestServerConfig) SetIBFTPoS(value bool) {
	t.IsPos = value
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

// SetBlockLimit sets the block gas limit
func (t *TestServerConfig) SetBlockLimit(limit uint64) {
	t.BlockGasLimit = limit
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
