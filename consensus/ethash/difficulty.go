package ethash

import (
	"github.com/umbracle/minimal/types"
)

const minDiff = 131072

// ConstantinopleBombDelay is the bomb delay for the Constantinople fork
const ConstantinopleBombDelay = 5000000

// ByzantiumBombDelay is the bomb delay for the Byzantium fork
const ByzantiumBombDelay = 3000000

// MetropolisDifficulty is the difficulty calculation for the metropolis forks
func MetropolisDifficulty(time int64, parent *types.Header, bombDelay uint64) uint64 {
	uncles := int64(1)
	if parent.Sha3Uncles != types.EmptyUncleHash {
		uncles = 2
	}
	var aux int64
	if val := uncles - (time-int64(parent.Timestamp))/9; val < -99 {
		aux = -99
	} else {
		aux = val
	}

	diff := calcAux(aux, parent.Difficulty)

	// bomb-delay
	period := uint64(0)
	if num := (parent.Number + 1); num >= bombDelay {
		period = num - bombDelay
	}
	if period := (period / 100000); period > 1 {
		diff += 1 << (period - 2)
	}
	return diff
}

func calcAux(aux int64, diff uint64) uint64 {
	// ((diff * 2048) * aux) + diff

	if aux > 0 {
		// aux is positive
		return (diff / 2048 * uint64(aux)) + diff
	}

	// aux is a negative.
	// We need to do: (a * -b) + c. Multiply uint and int
	// may overflow if the values are too big. Instead: c - (a * b)

	// remove the negative sign
	aux1 := uint64((aux ^ -1) + 1)

	res := (diff / 2048) * aux1
	if res > diff {
		// result is lower than zero
		return minDiff
	}

	res = diff - res
	if res < minDiff {
		res = minDiff
	}
	return res
}

// HomesteadDifficulty is the difficulty calculation for the homestead fork
func HomesteadDifficulty(time int64, parent *types.Header) uint64 {
	var aux int64
	if val := (1 - int64((time-int64(parent.Timestamp))/10)); val > -99 {
		aux = val
	} else {
		aux = -99
	}

	diff := calcAux(aux, parent.Difficulty)
	if period := ((parent.Number + 1) / 100000); period > 1 {
		diff += 1 << (period - 2)
	}
	return diff
}

// FrontierDifficulty is the difficulty calculation for the frontier fork
func FrontierDifficulty(time int64, parent *types.Header) uint64 {
	diff := parent.Difficulty
	aux := diff / 2048
	if time-int64(parent.Timestamp) < 13 {
		diff += aux
	} else {
		diff -= aux
	}

	if diff < minDiff {
		diff = minDiff
	}
	if period := ((parent.Number + 1) / 100000); period > 1 {
		diff += 1 << (period - 2)
	}
	return diff
}
