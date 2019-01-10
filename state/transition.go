package state

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/evm"
	newState "github.com/umbracle/minimal/state/state"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, homestead bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && homestead {
		gas = chain.TxGasContractCreation
	} else {
		gas = chain.TxGas
	}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		if (math.MaxUint64-gas)/chain.TxDataNonZeroGas < nz {
			return 0, vm.ErrOutOfGas
		}
		gas += nz * chain.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/chain.TxDataZeroGas < z {
			return 0, vm.ErrOutOfGas
		}
		gas += z * chain.TxDataZeroGas
	}
	return gas, nil
}

// Based on geth state_transition.go
type Transition struct {
	State      newState.State
	Env        *evm.Env
	GasTable   chain.GasTable
	Config     chain.ForksInTime
	Gas        uint64
	initialGas uint64
	Msg        *types.Message
	Gp         *core.GasPool
	GetHash    evm.GetHashByNumber
}

func (t *Transition) useGas(amount uint64) error {
	if t.Gas < amount {
		return vm.ErrOutOfGas
	}
	t.Gas -= amount

	return nil
}

func (t *Transition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(t.Msg.Gas()), t.Msg.GasPrice())
	if t.State.GetBalance(t.Msg.From()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := t.Gp.SubGas(t.Msg.Gas()); err != nil {
		return err
	}
	t.Gas += t.Msg.Gas()

	t.initialGas = t.Msg.Gas()
	t.State.SubBalance(t.Msg.From(), mgval)
	return nil
}

func (t *Transition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	if t.Msg.CheckNonce() {
		nonce := t.State.GetNonce(t.Msg.From())
		if nonce < t.Msg.Nonce() {
			return fmt.Errorf("too high %d < %d", nonce, t.Msg.Nonce())
		} else if nonce > t.Msg.Nonce() {
			return fmt.Errorf("too low %d > %d", nonce, t.Msg.Nonce())
		}
	}
	return t.buyGas()
}

func (t *Transition) Apply() error {
	if err := t.preCheck(); err != nil {
		return err
	}

	contractCreation := t.Msg.To() == nil

	sender := t.Msg.From()

	gas, err := IntrinsicGas(t.Msg.Data(), contractCreation, t.Config.Homestead)
	if err != nil {
		return err
	}

	if err = t.useGas(gas); err != nil {
		return err
	}

	e := evm.NewEVM(t.State, t.Env, t.Config, t.GasTable, t.GetHash)

	var vmerr error
	if contractCreation {
		_, t.Gas, vmerr = e.Create(sender, t.Msg.Data(), t.Msg.Value(), t.Gas)
	} else {
		t.State.SetNonce(t.Msg.From(), t.State.GetNonce(t.Msg.From())+1)
		_, t.Gas, vmerr = e.Call(sender, *t.Msg.To(), t.Msg.Data(), t.Msg.Value(), t.Gas)
	}

	if vmerr != nil {
		if vmerr == evm.ErrNotEnoughFunds {
			return vmerr
		}
	}

	t.refundGas()
	t.State.AddBalance(t.Env.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(t.gasUsed()), t.Msg.GasPrice()))

	return nil
}

func (t *Transition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := t.gasUsed() / 2

	if refund > t.State.GetRefund() {
		refund = t.State.GetRefund()
	}

	t.Gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(t.Gas), t.Msg.GasPrice())
	t.State.AddBalance(t.Msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	t.Gp.AddGas(t.Gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (t *Transition) gasUsed() uint64 {
	return t.initialGas - t.Gas
}
