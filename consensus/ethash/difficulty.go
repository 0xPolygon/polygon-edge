package ethash

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core/types"
)

var (
	big2     = big.NewInt(2)
	big2048  = big.NewInt(2048)
	minDiff  = big.NewInt(131072)
	bigNeg99 = big.NewInt(-99)
)

// ConstantinopleBombDelay is the bomb delay for the Constantinople fork
const ConstantinopleBombDelay = 5000000

// ByzantiumBombDelay is the bomb delay for the Byzantium fork
const ByzantiumBombDelay = 3000000

// MetropolisDifficulty is the difficulty calculation for the metropolis forks
func MetropolisDifficulty(time uint64, parent *types.Header, bombDelay uint64) *big.Int {
	diff := new(big.Int)
	aux := new(big.Int)

	uncles := int64(1)
	if parent.UncleHash != types.EmptyUncleHash {
		uncles = 2
	}
	if val := uncles - int64(time-parent.Time.Uint64())/9; val < -99 {
		aux.Set(bigNeg99)
	} else {
		aux.SetInt64(val)
	}

	diff.Div(parent.Difficulty, big2048)
	diff.Mul(diff, aux)
	diff.Add(diff, parent.Difficulty)

	if diff.Cmp(minDiff) < 0 {
		diff.Set(minDiff)
	}

	// bomb-delay
	period := uint64(0)
	if num := (parent.Number.Uint64() + 1); num >= bombDelay {
		period = num - bombDelay
	}
	if period := (period / 100000); period > 1 {
		aux.SetUint64(period - 2)
		aux.Exp(big2, aux, nil)
		diff.Add(diff, aux)
	}
	return diff
}

// HomesteadDifficulty is the difficulty calculation for the homestead fork
func HomesteadDifficulty(time uint64, parent *types.Header) *big.Int {
	diff := new(big.Int)
	aux := new(big.Int)

	if val := (1 - int64((time-parent.Time.Uint64())/10)); val > -99 {
		aux.SetInt64(val)
	} else {
		aux.Set(bigNeg99)
	}

	diff.Div(parent.Difficulty, big2048)
	diff.Mul(diff, aux)
	diff.Add(diff, parent.Difficulty)

	if diff.Cmp(minDiff) < 0 {
		diff.Set(minDiff)
	}

	if period := ((parent.Number.Uint64() + 1) / 100000); period > 1 {
		aux.SetUint64(period - 2)
		aux.Exp(big2, aux, nil)
		diff.Add(diff, aux)
	}
	return diff
}

// FrontierDifficulty is the difficulty calculation for the frontier fork
func FrontierDifficulty(time uint64, parent *types.Header) *big.Int {
	diff := new(big.Int).SetBytes(parent.Difficulty.Bytes())

	aux := big.NewInt(1).Div(diff, big2048)
	if time-parent.Time.Uint64() < 13 {
		diff.Add(diff, aux)
	} else {
		diff.Sub(diff, aux)
	}
	if diff.Cmp(minDiff) < 0 {
		diff.Set(minDiff)
	}

	if period := ((parent.Number.Uint64() + 1) / 100000); period > 1 {
		aux.SetUint64(period - 2)
		aux.Exp(big2, aux, nil)
		diff.Add(diff, aux)

		if diff.Cmp(minDiff) < 0 {
			diff.Set(minDiff)
		}
	}
	return diff
}
