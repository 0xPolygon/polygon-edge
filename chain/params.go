package chain

import (
	"errors"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/types"
)

var (
	// ErrBurnContractAddressMissing is the error when a contract address is not provided
	ErrBurnContractAddressMissing = errors.New("burn contract address missing")
)

// Params are all the set of params for the chain
type Params struct {
	Forks          *Forks                 `json:"forks"`
	ChainID        int64                  `json:"chainID"`
	Engine         map[string]interface{} `json:"engine"`
	Whitelists     *Whitelists            `json:"whitelists,omitempty"`
	BlockGasTarget uint64                 `json:"blockGasTarget"`

	// Access control configuration
	ContractDeployerAllowList *AddressListConfig `json:"contractDeployerAllowList,omitempty"`
	ContractDeployerBlockList *AddressListConfig `json:"contractDeployerBlockList,omitempty"`
	TransactionsAllowList     *AddressListConfig `json:"transactionsAllowList,omitempty"`
	TransactionsBlockList     *AddressListConfig `json:"transactionsBlockList,omitempty"`
	BridgeAllowList           *AddressListConfig `json:"bridgeAllowList,omitempty"`
	BridgeBlockList           *AddressListConfig `json:"bridgeBlockList,omitempty"`

	// Governance contract where the token will be sent to and burn in london fork
	BurnContract map[uint64]string `json:"burnContract"`
}

type AddressListConfig struct {
	// AdminAddresses is the list of the initial admin addresses
	AdminAddresses []types.Address `json:"adminAddresses,omitempty"`

	// EnabledAddresses is the list of the initial enabled addresses
	EnabledAddresses []types.Address `json:"enabledAddresses,omitempty"`
}

// CalculateBurnContract calculates burn contract address for the given block number
func (p *Params) CalculateBurnContract(block uint64) (types.Address, error) {
	blocks := make([]uint64, 0, len(p.BurnContract))

	for startBlock := range p.BurnContract {
		blocks = append(blocks, startBlock)
	}

	if len(blocks) == 0 {
		return types.ZeroAddress, ErrBurnContractAddressMissing
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i] < blocks[j]
	})

	for i := 0; i < len(blocks)-1; i++ {
		if block >= blocks[i] && block < blocks[i+1] {
			return types.StringToAddress(p.BurnContract[blocks[i]]), nil
		}
	}

	return types.StringToAddress(p.BurnContract[blocks[len(blocks)-1]]), nil
}

func (p *Params) GetEngine() string {
	// We know there is already one
	for k := range p.Engine {
		return k
	}

	return ""
}

// Whitelists specifies supported whitelists
type Whitelists struct {
	Deployment []types.Address `json:"deployment,omitempty"`
}

// Forks specifies when each fork is activated
type Forks struct {
	Homestead      *Fork `json:"homestead,omitempty"`
	Byzantium      *Fork `json:"byzantium,omitempty"`
	Constantinople *Fork `json:"constantinople,omitempty"`
	Petersburg     *Fork `json:"petersburg,omitempty"`
	Istanbul       *Fork `json:"istanbul,omitempty"`
	London         *Fork `json:"london,omitempty"`
	EIP150         *Fork `json:"EIP150,omitempty"`
	EIP158         *Fork `json:"EIP158,omitempty"`
	EIP155         *Fork `json:"EIP155,omitempty"`
}

func (f *Forks) active(ff *Fork, block uint64) bool {
	if ff == nil {
		return false
	}

	return ff.Active(block)
}

func (f *Forks) IsHomestead(block uint64) bool {
	return f.active(f.Homestead, block)
}

func (f *Forks) IsByzantium(block uint64) bool {
	return f.active(f.Byzantium, block)
}

func (f *Forks) IsConstantinople(block uint64) bool {
	return f.active(f.Constantinople, block)
}

func (f *Forks) IsPetersburg(block uint64) bool {
	return f.active(f.Petersburg, block)
}

func (f *Forks) IsLondon(block uint64) bool {
	return f.active(f.London, block)
}

func (f *Forks) IsEIP150(block uint64) bool {
	return f.active(f.EIP150, block)
}

func (f *Forks) IsEIP158(block uint64) bool {
	return f.active(f.EIP158, block)
}

func (f *Forks) IsEIP155(block uint64) bool {
	return f.active(f.EIP155, block)
}

func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
		Homestead:      f.active(f.Homestead, block),
		Byzantium:      f.active(f.Byzantium, block),
		Constantinople: f.active(f.Constantinople, block),
		Petersburg:     f.active(f.Petersburg, block),
		Istanbul:       f.active(f.Istanbul, block),
		London:         f.active(f.London, block),
		EIP150:         f.active(f.EIP150, block),
		EIP158:         f.active(f.EIP158, block),
		EIP155:         f.active(f.EIP155, block),
	}
}

type Fork uint64

func NewFork(n uint64) *Fork {
	f := Fork(n)

	return &f
}

func (f Fork) Active(block uint64) bool {
	return block >= uint64(f)
}

func (f Fork) Int() *big.Int {
	return big.NewInt(int64(f))
}

type ForksInTime struct {
	Homestead,
	Byzantium,
	Constantinople,
	Petersburg,
	Istanbul,
	London,
	EIP150,
	EIP158,
	EIP155 bool
}

var AllForksEnabled = &Forks{
	Homestead:      NewFork(0),
	EIP150:         NewFork(0),
	EIP155:         NewFork(0),
	EIP158:         NewFork(0),
	Byzantium:      NewFork(0),
	Constantinople: NewFork(0),
	Petersburg:     NewFork(0),
	Istanbul:       NewFork(0),
	London:         NewFork(0),
}
