package system

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/types"
)

var (
	OffsetNumValidators = types.ZeroHash
)

var one = big.NewInt(1)

// stakingHandler implements the staking logic for the System runtime
type stakingHandler struct {
	s *System
}

// gas returns the fixed gas price of the staking operation
func (sh *stakingHandler) gas(_ []byte) uint64 {
	fmt.Print("\n\n[Staking Handler] Gas calculation called\n\n")

	return 40000
}

// run executes the system contract staking method
func (sh *stakingHandler) run(state *systemState, config *chain.ForksInTime) ([]byte, error) {
	fmt.Printf("\n\n[Staking Handler RUN]\n\n [STAKER]: %s\n[AMOUNT]: %s\n\n", state.contract.Caller, state.contract.Value)

	// Grab the value being staked
	potentialStake := state.contract.Value

	// Grab the address calling the staking method
	staker := state.contract.Caller

	// Increase the account's staked balance
	state.host.AddStakedBalance(staker, potentialStake)

	if !IsValidator(state.host, staker) {
		appendValidator(state.host, staker, config)
	}

	return nil, nil
}

func IsValidator(h runtime.Host, addr types.Address) bool {
	num := GetNumValidators(h)
	for i := big.NewInt(0); i.Cmp(num) < 0; i.Add(i, one) {
		validator := GetValidatorAt(h, i)
		if bytes.Equal(addr[:], validator[:]) {
			return true
		}
	}
	return false
}

func GetNumValidators(h runtime.Host) *big.Int {
	numBytes := h.GetStorage(types.StringToAddress(AddrStaking), OffsetNumValidators)
	return new(big.Int).SetBytes(numBytes[:])
}

func GetValidators(h runtime.Host) []types.Address {
	num := GetNumValidators(h)
	addrs := make([]types.Address, 0, num.Int64())
	for i := big.NewInt(0); i.Cmp(num) < 0; i.Add(i, one) {
		addr := GetValidatorAt(h, i)
		addrs = append(addrs, addr)
	}
	return addrs
}

func GetValidatorAt(h runtime.Host, index *big.Int) types.Address {
	ptr := getValidatorPtr(index)
	value := h.GetStorage(types.StringToAddress(AddrStaking), ptr)
	return types.BytesToAddress(value[:])
}

func appendValidator(h runtime.Host, addr types.Address, config *chain.ForksInTime) {
	oldNum := GetNumValidators(h)
	newNum := new(big.Int).Add(oldNum, one)
	// update number of validators
	h.SetStorage(types.StringToAddress(AddrStaking), OffsetNumValidators, types.BytesToHash(newNum.Bytes()), config)
	setValidatorAt(h, oldNum, addr, config)
}

func setValidatorAt(h runtime.Host, index *big.Int, addr types.Address, config *chain.ForksInTime) {
	ptr := getValidatorPtr(index)
	h.SetStorage(types.StringToAddress(AddrStaking), ptr, types.BytesToHash(addr[:]), config)
}

func getValidatorPtr(index *big.Int) types.Hash {
	offset := new(big.Int).Add(new(big.Int).SetBytes(OffsetNumValidators[:]), one)
	ptrBytes := new(big.Int).Add(offset, index).Bytes()
	return types.BytesToHash(ptrBytes)
}
