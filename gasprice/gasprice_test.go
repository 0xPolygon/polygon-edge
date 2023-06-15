package gasprice

import (
	"math/big"
	"sort"
	"testing"
)

func TestBigIntSorter(t *testing.T) {
	t.Parallel()

	bigInts := []*big.Int{big.NewInt(10), big.NewInt(2), big.NewInt(15), big.NewInt(5)}
	sort.Sort(bigIntSorted(bigInts))
}
