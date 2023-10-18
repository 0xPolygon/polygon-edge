package framework

import (
	"crypto/ecdsa"
	"math/big"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
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

type PredeployParams struct {
	ArtifactsPath    string
	PredeployAddress string
	ConstructorArgs  []string
}

// TestServerConfig for the test server
type TestServerConfig struct {
	ReservedPorts           []ReservedPort
	JSONRPCPort             int                      // The JSON RPC endpoint port
	GRPCPort                int                      // The GRPC endpoint port
	LibP2PPort              int                      // The Libp2p endpoint port
	RootDir                 string                   // The root directory for test environment
	IBFTDirPrefix           string                   // The prefix of data directory for IBFT
	IBFTDir                 string                   // The name of data directory for IBFT
	PremineAccts            []*SrvAccount            // Accounts with existing balances (genesis accounts)
	GenesisValidatorBalance *big.Int                 // Genesis the balance for the validators
	DevStakers              []types.Address          // List of initial staking addresses for the staking SC
	Consensus               ConsensusType            // Consensus MechanismType
	ValidatorType           validators.ValidatorType // Validator Type
	Bootnodes               []string                 // Bootnode Addresses
	PriceLimit              *uint64                  // Minimum gas price limit to enforce for acceptance into the pool
	DevInterval             int                      // Dev consensus update interval [s]
	EpochSize               uint64                   // The epoch size in blocks for the IBFT layer
	BlockGasLimit           uint64                   // Block gas limit
	BlockGasTarget          uint64                   // Gas target for new blocks
	BaseFee                 uint64                   // Initial base fee
	ShowsLog                bool                     // Flag specifying if logs are shown
	Name                    string                   // Name of the server
	SaveLogs                bool                     // Flag specifying if logs are saved
	LogsDir                 string                   // Directory where logs are saved
	IsPos                   bool                     // Specifies the mechanism used for IBFT (PoA / PoS)
	Signer                  crypto.TxSigner          // Signer used for transactions
	MinValidatorCount       uint64                   // Min validator count
	MaxValidatorCount       uint64                   // Max validator count
	BlockTime               uint64                   // Minimum block generation time (in s)
	IBFTBaseTimeout         uint64                   // Base Timeout in seconds for IBFT
	PredeployParams         *PredeployParams
	BurnContracts           map[uint64]types.Address
}

func (t *TestServerConfig) SetPredeployParams(params *PredeployParams) {
	t.PredeployParams = params
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

func (t *TestServerConfig) SetSigner(signer crypto.TxSigner) {
	t.Signer = signer
}

func (t *TestServerConfig) SetBlockTime(blockTime uint64) {
	t.BlockTime = blockTime
}

func (t *TestServerConfig) SetIBFTBaseTimeout(baseTimeout uint64) {
	t.IBFTBaseTimeout = baseTimeout
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

// SetBurnContract sets the given burn contract for the test server
func (t *TestServerConfig) SetBurnContract(block uint64, address types.Address) {
	if t.BurnContracts == nil {
		t.BurnContracts = map[uint64]types.Address{}
	}

	t.BurnContracts[block] = address
}

// SetConsensus callback sets consensus
func (t *TestServerConfig) SetConsensus(c ConsensusType) {
	t.Consensus = c
}

// SetValidatorType callback sets validator type
func (t *TestServerConfig) SetValidatorType(vt validators.ValidatorType) {
	t.ValidatorType = vt
}

// SetDevInterval sets the update interval for the dev consensus
func (t *TestServerConfig) SetDevInterval(interval int) {
	t.DevInterval = interval
}

// SetDevStakingAddresses sets the Staking smart contract staker addresses for the dev mode.
// These addresses should be passed into the `validators` flag in genesis generation.
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

// SetBootnodes sets bootnodes
func (t *TestServerConfig) SetBootnodes(bootnodes []string) {
	t.Bootnodes = bootnodes
}

// SetPriceLimit sets the gas price limit
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

// SetMinValidatorCount sets the min validator count
func (t *TestServerConfig) SetMinValidatorCount(val uint64) {
	t.MinValidatorCount = val
}

// SetMaxValidatorCount sets the max validator count
func (t *TestServerConfig) SetMaxValidatorCount(val uint64) {
	t.MaxValidatorCount = val
}

// SetSaveLogs sets flag for saving logs
func (t *TestServerConfig) SetSaveLogs(f bool) {
	t.SaveLogs = f
}

// SetLogsDir sets the directory where logs are saved
func (t *TestServerConfig) SetLogsDir(dir string) {
	t.LogsDir = dir
}

// SetName sets the name of the server
func (t *TestServerConfig) SetName(name string) {
	t.Name = name
}
