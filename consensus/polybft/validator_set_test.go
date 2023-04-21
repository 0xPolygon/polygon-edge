package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestValidatorSet_HasQuorum(t *testing.T) {
	t.Parallel()

	// enough signers for quorum (2/3 super-majority of validators are signers)
	validators := newTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F", "G"})
	vs := validators.toValidatorSet()

	signers := make(map[types.Address]struct{})

	validators.iterAcct([]string{"A", "B", "C", "D", "E"}, func(v *testValidator) {
		signers[v.Address()] = struct{}{}
	})

	require.True(t, vs.HasQuorum(signers))

	// not enough signers for quorum (less than 2/3 super-majority of validators are signers)
	signers = make(map[types.Address]struct{})

	validators.iterAcct([]string{"A", "B", "C", "D"}, func(v *testValidator) {
		signers[v.Address()] = struct{}{}
	})
	require.False(t, vs.HasQuorum(signers))
}

func TestValidatorSet_getQuorumSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		totalVotingPower   int64
		expectedQuorumSize int64
	}{
		{10, 7},
		{12, 8},
		{13, 9},
		{50, 34},
		{100, 67},
	}

	for _, c := range cases {
		quorumSize := getQuorumSize(big.NewInt(c.totalVotingPower))
		require.Equal(t, c.expectedQuorumSize, quorumSize.Int64())
	}
}
