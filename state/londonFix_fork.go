package state

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

const LondonFixHandler forkmanager.HandlerDesc = "LondonFixHandler"

type LondonFixFork interface {
	checkDynamicFees(*types.Transaction, *Transition) error
	getUpfrontGasCost(msg *types.Transaction, baseFee *big.Int) *big.Int
	getEffectiveTip(msg *types.Transaction, gasPrice *big.Int,
		baseFee *big.Int, isLondonForkEnabled bool) *big.Int
}

type LondonFixForkV1 struct{}

// checkDynamicFees checks correctness of the EIP-1559 feature-related fields.
// Basically, makes sure gas tip cap and gas fee cap are good for dynamic and legacy transactions
// and that GasFeeCap/GasPrice cap is not lower than base fee when London fork is active.
func (l *LondonFixForkV1) checkDynamicFees(msg *types.Transaction, t *Transition) error {
	if msg.Type != types.DynamicFeeTx {
		return nil
	}

	if msg.GasFeeCap.BitLen() == 0 && msg.GasTipCap.BitLen() == 0 {
		return nil
	}

	if l := msg.GasFeeCap.BitLen(); l > 256 {
		return fmt.Errorf("%w: address %v, GasFeeCap bit length: %d", ErrFeeCapVeryHigh,
			msg.From.String(), l)
	}

	if l := msg.GasTipCap.BitLen(); l > 256 {
		return fmt.Errorf("%w: address %v, GasTipCap bit length: %d", ErrTipVeryHigh,
			msg.From.String(), l)
	}

	if msg.GasFeeCap.Cmp(msg.GasTipCap) < 0 {
		return fmt.Errorf("%w: address %v, GasTipCap: %s, GasFeeCap: %s", ErrTipAboveFeeCap,
			msg.From.String(), msg.GasTipCap, msg.GasFeeCap)
	}

	// This will panic if baseFee is nil, but basefee presence is verified
	// as part of header validation.
	if msg.GasFeeCap.Cmp(t.ctx.BaseFee) < 0 {
		return fmt.Errorf("%w: address %v, GasFeeCap: %s, BaseFee: %s", ErrFeeCapTooLow,
			msg.From.String(), msg.GasFeeCap, t.ctx.BaseFee)
	}

	return nil
}

func (l *LondonFixForkV1) getUpfrontGasCost(msg *types.Transaction, baseFee *big.Int) *big.Int {
	upfrontGasCost := new(big.Int).SetUint64(msg.Gas)

	factor := new(big.Int)
	if msg.GasFeeCap != nil && msg.GasFeeCap.BitLen() > 0 {
		// Apply EIP-1559 tx cost calculation factor
		factor = factor.Set(msg.GasFeeCap)
	} else {
		// Apply legacy tx cost calculation factor
		factor = factor.Set(msg.GasPrice)
	}

	return upfrontGasCost.Mul(upfrontGasCost, factor)
}

func (l *LondonFixForkV1) getEffectiveTip(msg *types.Transaction, gasPrice *big.Int,
	baseFee *big.Int, isLondonForkEnabled bool) *big.Int {
	if isLondonForkEnabled && msg.Type == types.DynamicFeeTx {
		return common.BigMin(
			new(big.Int).Sub(msg.GasFeeCap, baseFee),
			new(big.Int).Set(msg.GasTipCap),
		)
	}

	return new(big.Int).Set(gasPrice)
}

type LondonFixForkV2 struct{}

func (l *LondonFixForkV2) checkDynamicFees(msg *types.Transaction, t *Transition) error {
	if !t.config.London {
		return nil
	}

	if msg.Type == types.DynamicFeeTx {
		if msg.GasFeeCap.BitLen() == 0 && msg.GasTipCap.BitLen() == 0 {
			return nil
		}

		if l := msg.GasFeeCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, GasFeeCap bit length: %d", ErrFeeCapVeryHigh,
				msg.From.String(), l)
		}

		if l := msg.GasTipCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, GasTipCap bit length: %d", ErrTipVeryHigh,
				msg.From.String(), l)
		}

		if msg.GasFeeCap.Cmp(msg.GasTipCap) < 0 {
			return fmt.Errorf("%w: address %v, GasTipCap: %s, GasFeeCap: %s", ErrTipAboveFeeCap,
				msg.From.String(), msg.GasTipCap, msg.GasFeeCap)
		}
	}

	// This will panic if baseFee is nil, but basefee presence is verified
	// as part of header validation.
	if gasFeeCap := msg.GetGasFeeCap(); gasFeeCap.Cmp(t.ctx.BaseFee) < 0 {
		return fmt.Errorf("%w: address %v, GasFeeCap/GasPrice: %s, BaseFee: %s", ErrFeeCapTooLow,
			msg.From.String(), gasFeeCap, t.ctx.BaseFee)
	}

	return nil
}

func (l *LondonFixForkV2) getUpfrontGasCost(msg *types.Transaction, baseFee *big.Int) *big.Int {
	return new(big.Int).Mul(new(big.Int).SetUint64(msg.Gas), msg.GetGasPrice(baseFee.Uint64()))
}

func (l *LondonFixForkV2) getEffectiveTip(msg *types.Transaction, gasPrice *big.Int,
	baseFee *big.Int, isLondonForkEnabled bool) *big.Int {
	if isLondonForkEnabled {
		return msg.EffectiveGasTip(baseFee)
	}

	return new(big.Int).Set(gasPrice)
}

func RegisterLondonFixFork(londonFixFork string) error {
	fh := forkmanager.GetInstance()

	if err := fh.RegisterHandler(
		forkmanager.InitialFork, LondonFixHandler, &LondonFixForkV1{}); err != nil {
		return err
	}

	if fh.IsForkRegistered(londonFixFork) {
		if err := fh.RegisterHandler(
			londonFixFork, LondonFixHandler, &LondonFixForkV2{}); err != nil {
			return err
		}
	}

	return nil
}

func GetLondonFixHandler(blockNumber uint64) LondonFixFork {
	if h := forkmanager.GetInstance().GetHandler(LondonFixHandler, blockNumber); h != nil {
		//nolint:forcetypeassert
		return h.(LondonFixFork)
	}

	// for tests
	return &LondonFixForkV2{}
}
