package state

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/types"
)

const LondonFixHandler forkmanager.HandlerDesc = "LondonFixHandler"

type LondonFixFork interface {
	checkDynamicFees(*types.Transaction, *Transition) error
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
	if msg.GetGasFeeCap().Cmp(t.ctx.BaseFee) < 0 {
		return fmt.Errorf("%w: address %v, GasFeeCap: %s, BaseFee: %s", ErrFeeCapTooLow,
			msg.From.String(), msg.GasFeeCap, t.ctx.BaseFee)
	}

	return nil
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
	return &LondonFixForkV1{}
}
