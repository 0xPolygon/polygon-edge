package ibft

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestState_FaultyNodes(t *testing.T) {
	cases := []struct {
		Network, Faulty uint64
	}{
		{1, 0},
		{2, 0},
		{3, 0},
		{4, 1},
		{5, 1},
		{6, 1},
		{7, 2},
		{8, 2},
		{9, 2},
	}
	for _, c := range cases {
		pool := newTesterAccountPool(t, int(c.Network))
		vals := pool.ValidatorSet()
		assert.Equal(t, CalcMaxFaultyNodes(vals), int(c.Faulty))
	}
}

// TestNumValid checks if the quorum size is calculated
// correctly based on number of validators (network size).
func TestNumValid(t *testing.T) {
	cases := []struct {
		Network, Quorum uint64
	}{
		{1, 1},
		{2, 2},
		{3, 3},
		{4, 3},
		{5, 4},
		{6, 4},
		{7, 5},
		{8, 6},
		{9, 6},
	}

	addAccounts := func(
		pool *testerAccountPool,
		numAccounts int,
	) {
		// add accounts
		for i := 0; i < numAccounts; i++ {
			pool.add(strconv.Itoa(i))
		}
	}

	for _, c := range cases {
		pool := newTesterAccountPool(t, int(c.Network))
		addAccounts(pool, int(c.Network))

		assert.Equal(t,
			int(c.Quorum),
			OptimalQuorumSize(pool.ValidatorSet()),
		)
	}
}
