package chain

import (
	"errors"
	"math/big"
	"reflect"
	"sort"
	"strings"

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

// predefined forks
const (
	Homestead      = "homestead"
	Byzantium      = "byzantium"
	Constantinople = "constantinople"
	Petersburg     = "petersburg"
	Istanbul       = "istanbul"
	London         = "london"
	EIP150         = "EIP150"
	EIP158         = "EIP158"
	EIP155         = "EIP155"
)

type AvailableForks struct {
	Homestead      *Fork
	Byzantium      *Fork
	Constantinople *Fork
	Petersburg     *Fork
	Istanbul       *Fork
	London         *Fork
	EIP150         *Fork
	EIP158         *Fork
	EIP155         *Fork
}

func (f *AvailableForks) At(block uint64) ForksInTime {
	return ForksInTime{
		Homestead:      active(f.Homestead, block),
		Byzantium:      active(f.Byzantium, block),
		Constantinople: active(f.Constantinople, block),
		Petersburg:     active(f.Petersburg, block),
		Istanbul:       active(f.Istanbul, block),
		London:         active(f.London, block),
		EIP150:         active(f.EIP150, block),
		EIP158:         active(f.EIP158, block),
		EIP155:         active(f.EIP155, block),
	}
}

func (f *AvailableForks) ToForks() *Forks {
	forks := &Forks{}

	add := func(name string, fork *Fork) {
		if fork != nil {
			(*forks)[name] = fork
		}
	}

	add(Homestead, f.Homestead)
	add(Byzantium, f.Byzantium)
	add(Constantinople, f.Constantinople)
	add(Petersburg, f.Petersburg)
	add(Istanbul, f.Istanbul)
	add(London, f.London)
	add(EIP150, f.EIP150)
	add(EIP158, f.EIP158)
	add(EIP155, f.EIP155)

	return forks
}

// Forks specifies when each fork is activated
type Forks map[string]*Fork

func (f *Forks) IsHomestead(block uint64) bool {
	return active((*f)[Homestead], block)
}

func (f *Forks) IsByzantium(block uint64) bool {
	return active((*f)[Byzantium], block)
}

func (f *Forks) IsConstantinople(block uint64) bool {
	return active((*f)[Constantinople], block)
}

func (f *Forks) IsPetersburg(block uint64) bool {
	return active((*f)[Petersburg], block)
}

func (f *Forks) IsLondon(block uint64) bool {
	return active((*f)[London], block)
}

func (f *Forks) IsEIP150(block uint64) bool {
	return active((*f)[EIP150], block)
}

func (f *Forks) IsEIP158(block uint64) bool {
	return active((*f)[EIP158], block)
}

func (f *Forks) IsEIP155(block uint64) bool {
	return active((*f)[EIP155], block)
}

func (f *Forks) Is(name string, block uint64) bool {
	return active((*f)[name], block)
}

func (f *Forks) IsSupported(name string) bool {
	_, exists := (*f)[name]

	return exists
}

func (f *Forks) At(block uint64) ForksInTime {
	return ForksInTime{
		Homestead:      active((*f)[Homestead], block),
		Byzantium:      active((*f)[Byzantium], block),
		Constantinople: active((*f)[Constantinople], block),
		Petersburg:     active((*f)[Petersburg], block),
		Istanbul:       active((*f)[Istanbul], block),
		London:         active((*f)[London], block),
		EIP150:         active((*f)[EIP150], block),
		EIP158:         active((*f)[EIP158], block),
		EIP155:         active((*f)[EIP155], block),
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

var AllForksEnabled = &AvailableForks{
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

func active(ff *Fork, block uint64) bool {
	if ff == nil {
		return false
	}

	return ff.Active(block)
}

func IsForkAvailable(name string) bool {
	structureType := reflect.TypeOf(AvailableForks{})
	_, found := structureType.FieldByName(strings.Title(name))

	return found
}
