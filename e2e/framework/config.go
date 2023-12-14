package framework

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
)

type ConsensusType int

const (
	ConsensusDev ConsensusType = iota
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
	JSONRPCPort             int             // The JSON RPC endpoint port
	GRPCPort                int             // The GRPC endpoint port
	LibP2PPort              int             // The Libp2p endpoint port
	RootDir                 string          // The root directory for test environment
	PremineAccts            []*SrvAccount   // Accounts with existing balances (genesis accounts)
	GenesisValidatorBalance *big.Int        // Genesis the balance for the validators
	Consensus               ConsensusType   // Consensus MechanismType
	Bootnodes               []string        // Bootnode Addresses
	PriceLimit              *uint64         // Minimum gas price limit to enforce for acceptance into the pool
	DevInterval             int             // Dev consensus update interval [s]
	BlockGasLimit           uint64          // Block gas limit
	BlockGasTarget          uint64          // Gas target for new blocks
	BurnContractAddr        types.Address   // Burn contract addr
	ShowsLog                bool            // Flag specifying if logs are shown
	Name                    string          // Name of the server
	SaveLogs                bool            // Flag specifying if logs are saved
	LogsDir                 string          // Directory where logs are saved
	Signer                  crypto.TxSigner // Signer used for transactions
}

// DataDir returns path of data directory server uses
func (t *TestServerConfig) DataDir() string {
	return t.RootDir
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

// SetBurnContract sets burn contract addr
func (t *TestServerConfig) SetBurnContract(addr types.Address) {
	t.BurnContractAddr = addr
}
