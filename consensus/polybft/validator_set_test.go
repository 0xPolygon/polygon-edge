package polybft

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestValidatorSet_HasQuorum(t *testing.T) {
	t.Parallel()

	// enough signers for quorum (2/3 super-majority of validators are signers)
	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F", "G"})
	vs, err := validators.toValidatorSet()
	require.NoError(t, err)

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
