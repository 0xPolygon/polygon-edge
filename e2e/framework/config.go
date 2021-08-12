package framework

import (
	"crypto/ecdsa"
	"math/big"
	"path/filepath"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/network"
	"github.com/0xPolygon/minimal/types"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
)

type ConsensusType int

const (
	ConsensusIBFT ConsensusType = iota
	ConsensusDev
	ConsensusDummy
)

type SrvAccount struct {
	Addr          types.Address
	Balance       *big.Int
	StakedBalance *big.Int
}

type GenesisValidatorBalance struct {
	Balance       *big.Int
	StakedBalance *big.Int
}

// TestServerConfig for the test server
type TestServerConfig struct {
	ReservedPorts           []ReservedPort
	JsonRPCPort             int                      // The JSON RPC endpoint port
	GRPCPort                int                      // The GRPC endpoint port
	LibP2PPort              int                      // The Libp2p endpoint port
	Seal                    bool                     // Flag indicating if blocks should be sealed
	RootDir                 string                   // The root directory for test environment
	IBFTDirPrefix           string                   // The prefix of data directory for IBFT
	IBFTDir                 string                   // The name of data directory for IBFT
	PremineAccts            []*SrvAccount            // Accounts with existing balances (genesis accounts)
	GenesisValidatorBalance *GenesisValidatorBalance // Genesis balance for the validator
	Consensus               ConsensusType            // Consensus Type
	Bootnodes               []string                 // Bootnode Addresses
	DevInterval             int                      // Dev consensus update interval [s]
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

// Libp2pKey returns a private key for libp2p in data directory
func (t *TestServerConfig) Libp2pKey() (libp2pCrypto.PrivKey, error) {
	return network.ReadLibp2pKey(filepath.Join(t.DataDir(), "libp2p"))
}

// CALLBACKS //

// Premine callback specifies an account with a balance (in WEI)
func (t *TestServerConfig) Premine(addr types.Address, amount *big.Int) {
	if t.PremineAccts == nil {
		t.PremineAccts = []*SrvAccount{}
	}
	t.PremineAccts = append(t.PremineAccts, &SrvAccount{
		Addr:          addr,
		Balance:       amount,
		StakedBalance: big.NewInt(0),
	})
}

// PremineWithStake callback specifies an account with a balance, as well as a staked balance (in WEI)
func (t *TestServerConfig) PremineWithStake(addr types.Address, amount *big.Int, stakedAmount *big.Int) {
	if t.PremineAccts == nil {
		t.PremineAccts = []*SrvAccount{}
	}
	t.PremineAccts = append(t.PremineAccts, &SrvAccount{
		Addr:          addr,
		Balance:       amount,
		StakedBalance: stakedAmount,
	})
}

// SetPremineValidatorBalance callback set genesis balance of the validator the server manages (in WEI)
func (t *TestServerConfig) PremineValidatorBalance(balance, stakedBalance *big.Int) {
	t.GenesisValidatorBalance = &GenesisValidatorBalance{
		Balance:       balance,
		StakedBalance: stakedBalance,
	}
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
