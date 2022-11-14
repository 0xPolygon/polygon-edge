package polybft

import (
	"bytes"
	"math"
	"testing"
	"testing/quick"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValSetIndex(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(5)
	require.NoError(t, err)

	addresses := []types.Address{{0x10}, {0x52}, {0x33}, {0x74}, {0x60}}

	vs, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     addresses[0],
			VotingPower: 10,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     addresses[1],
			VotingPower: 100,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     addresses[2],
			VotingPower: 1,
		},
		{
			BlsKey:      keys[3].PublicKey(),
			Address:     addresses[3],
			VotingPower: 50,
		},
		{
			BlsKey:      keys[4].PublicKey(),
			Address:     addresses[4],
			VotingPower: 30,
		},
	}, hclog.NewNullLogger())
	require.NoError(t, err)
	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, addresses[i], v.Address)
	}

	proposer, err := vs.GetProposer()
	require.NoError(t, err)

	address := proposer.Metadata.Address
	assert.Equal(t, address, addresses[1])

	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, addresses[i], v.Address)
	}
}

func TestCalculateProposer(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(5)
	require.NoError(t, err)

	vs, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: 1,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: 2,
		},
		{
			BlsKey:      keys[3].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: 3,
		},
		{
			BlsKey:      keys[4].PublicKey(),
			Address:     types.Address{0x4},
			VotingPower: 4,
		},
		{
			BlsKey:      keys[4].PublicKey(),
			Address:     types.Address{0x5},
			VotingPower: 5,
		},
	}, hclog.NewNullLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(15), vs.totalVotingPower)

	curr, err := vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x5}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x4}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x5}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x4}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, curr.Metadata.Address)
}

func TestCalcProposer(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(5)
	require.NoError(t, err)

	vs, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: 1,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: 2,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: 3,
		},
	}, hclog.NewNullLogger())
	require.NoError(t, err)

	proposer, err := vs.CalcProposer(0)
	require.NoError(t, err)
	assert.Equal(t, "0x0300000000000000000000000000000000000000", proposer.String())

	proposer, err = vs.CalcProposer(1)
	require.NoError(t, err)
	assert.Equal(t, "0x0200000000000000000000000000000000000000", proposer.String())

	proposer, err = vs.CalcProposer(2)
	require.NoError(t, err)
	assert.Equal(t, "0x0100000000000000000000000000000000000000", proposer.String())
}

func TestProposerSelection1(t *testing.T) {
	t.Parallel()

	const numberOfIteration = 99

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	vset, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: 1000,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: 300,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: 330,
		},
	}, hclog.NewNullLogger())
	require.NoError(t, err)

	var proposers = make([]types.Address, numberOfIteration)

	for i := 0; i < numberOfIteration; i++ {
		val, err := vset.GetProposer()
		require.NoError(t, err)

		proposers[i] = val.Metadata.Address
		err = vset.IncrementProposerPriority(1)
		require.NoError(t, err)
	}

	expected := []types.Address{
		{0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1},
		{0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1},
		{0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3},
		{0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x1}, {0x3}, {0x2}, {0x1}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2},
		{0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1},
		{0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1},
		{0x3}, {0x1}, {0x1},
	}

	for i, p := range proposers {
		assert.True(t, bytes.Equal(expected[i].Bytes(), p.Bytes()))
	}
}

// Test that IncrementProposerPriority requires positive times.
func TestIncrementProposerPriorityPositiveTimes(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	vset, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: 1000,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: 300,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: 330,
		},
	}, hclog.NewNullLogger())

	require.NoError(t, err)
	proposer, err := vset.GetProposer()
	assert.Equal(t, types.Address{0x1}, proposer.Metadata.Address)

	err = vset.IncrementProposerPriority(0)
	require.Error(t, err)

	err = vset.IncrementProposerPriority(1)
	require.NoError(t, err)

	proposer, err = vset.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposer.Metadata.Address)
}

func TestIncrementProposerPrioritySameVotingPower(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	vs, err := NewValidatorSet([]*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: 1,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: 1,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: 1,
		},
	}, hclog.NewNullLogger())
	require.NoError(t, err)
	assert.Equal(t, int64(3), vs.totalVotingPower)

	// when voting power is the same order is by address
	curr, err := vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)
	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)

	curr, err = vs.GetProposer()
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, curr.Metadata.Address)

	err = vs.IncrementProposerPriority(1)
	require.NoError(t, err)
}

func TestAveragingInIncrementProposerPriorityWithVotingPower(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	// Tests how each ProposerPriority changes in relation to the validator's voting power respectively.
	// average is zero in each round:
	vp0 := uint64(10)
	vp1 := uint64(1)
	vp2 := uint64(1)
	total := vp0 + vp1 + vp2
	avg := (vp0 + vp1 + vp2 - total) / 3 // avg is used to center priorities around zero

	// in every iteration expected proposer is the one with the highest priority based on the voting power
	// priority is calculated: priority = iterationNO * voting power, once node is selected total voting power
	// is subtracted from priority of selected node
	valz := []*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: vp0,
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: vp1,
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: vp2,
		},
	}

	vals, err := NewValidatorSet(valz, hclog.NewNullLogger())

	tcs := []struct {
		vals                 *validatorSet
		wantProposerPriories []uint64
		times                uint64
		wantProposer         *ValidatorMetadata
	}{

		0: {
			vals.Copy(),
			[]uint64{
				// Acumm+VotingPower-Avg:
				0 + vp0 - total - avg, // mostest will be subtracted by total voting power (12)
				0 + vp1,
				0 + vp2},
			1,
			vals.validators[0].Metadata},
		1: {
			vals.Copy(),
			[]uint64{
				0 + 2*(vp0-total) - avg, // this will be mostest on 2nd iter, too
				(0 + vp1) + vp1,
				(0 + vp2) + vp2},
			2,
			vals.validators[0].Metadata}, // increment twice -> expect average to be subtracted twice
		2: {
			vals.Copy(),
			[]uint64{
				0 + 3*(vp0-total) - avg, // 3rd iteration, still mostest
				0 + 3*vp1,
				0 + 3*vp2},
			3,
			vals.validators[0].Metadata},
		3: {
			vals.Copy(),
			[]uint64{
				0 + 4*(vp0-total), // 4th iteration still mostest
				0 + 4*vp1,
				0 + 4*vp2},
			4,
			vals.validators[0].Metadata},
		4: {
			vals.Copy(),
			[]uint64{
				0 + 4*(vp0-total) + vp0, // 4 iteration was mostest
				0 + 5*vp1 - total,       // 5th iteration this val is mostest for the 1st time (hence -12==totalVotingPower)
				0 + 5*vp2},
			5,
			vals.validators[1].Metadata},
		5: {
			vals.Copy(),
			[]uint64{
				0 + 6*vp0 - 5*total, // 6th iteration mostest again
				0 + 6*vp1 - total,   // mostest once up to here
				0 + 6*vp2},
			6,
			vals.validators[0].Metadata},
		6: {
			vals.Copy(),
			[]uint64{
				0 + 7*vp0 - 6*total, // in 7 iteration this val is mostest 6 times
				0 + 7*vp1 - total,   // in 7 iteration this val is mostest 1 time
				0 + 7*vp2},
			7,
			vals.validators[0].Metadata},
		7: {
			vals.Copy(),
			[]uint64{
				0 + 8*vp0 - 7*total, // 8th iteration mostest again is picked
				0 + 8*vp1 - total,
				0 + 8*vp2},
			8,
			vals.validators[0].Metadata},
		8: {
			vals.Copy(),
			[]uint64{
				0 + 9*vp0 - 7*total,
				0 + 9*vp1 - total,
				0 + 9*vp2 - total}, // 9th iteration and now first time mostest is picked
			9,
			vals.validators[2].Metadata},
		9: {
			vals.Copy(),
			[]uint64{
				0 + 10*vp0 - 8*total, // after 10 iterations this is mostest again
				0 + 10*vp1 - total,   // after 6 iterations this val is "mostest" once and not in between
				0 + 10*vp2 - total},  // in between 10 iterations this val is "mostest" once
			10,
			vals.validators[0].Metadata},
		10: {
			vals.Copy(),
			[]uint64{
				0 + 11*vp0 - 9*total, // 11th iteration again is picked
				0 + 11*vp1 - total,   // after 6 iterations this val is "mostest" once and not in between
				0 + 11*vp2 - total},  // after 10 iterations this val is "mostest" once
			11,
			vals.validators[0].Metadata},
	}

	for i, tc := range tcs {
		err := tc.vals.IncrementProposerPriority(tc.times)
		require.NoError(t, err)
		proposer, err := tc.vals.GetProposer()
		require.NoError(t, err)
		assert.Equal(t, tc.wantProposer.Address, proposer.Metadata.Address,
			"test case: %v",
			i)

		for valIdx, val := range tc.vals.validators {
			assert.Equal(t,
				tc.wantProposerPriories[valIdx],
				uint64(val.ProposerPriority),
				"test case: %v, validator: %v",
				i,
				valIdx)
		}
	}
}

func TestValidatorSetTotalVotingPowerPanicsOnOverflow(t *testing.T) {
	t.Parallel()

	// NewValidatorSet calls IncrementProposerPriority which calls TotalVotingPower()
	// which should panic on overflows:
	vs, err := NewValidatorSet([]*ValidatorMetadata{
		{Address: types.Address{0x1}, VotingPower: math.MaxInt64},
		{Address: types.Address{0x2}, VotingPower: math.MaxInt64},
		{Address: types.Address{0x3}, VotingPower: math.MaxInt64},
	}, hclog.NewNullLogger())
	require.Error(t, err)

	err = vs.IncrementProposerPriority(1)
	require.Error(t, err)
}

func TestSafeAdd(t *testing.T) {
	t.Parallel()

	f := func(a, b int64) bool {
		c, overflow := safeAdd(a, b)

		return overflow || (!overflow && c == a+b)
	}
	if err := quick.Check(f, nil); err != nil {
		t.Error(err)
	}
}

func TestSafeAddClip(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, math.MaxInt64, safeAddClip(math.MaxInt64, 10))
	assert.EqualValues(t, math.MaxInt64, safeAddClip(math.MaxInt64, math.MaxInt64))
	assert.EqualValues(t, math.MinInt64, safeAddClip(math.MinInt64, -10))
}

func TestSafeSubClip(t *testing.T) {
	t.Parallel()

	assert.EqualValues(t, math.MinInt64, safeSubClip(math.MinInt64, 10))
	assert.EqualValues(t, 0, safeSubClip(math.MinInt64, math.MinInt64))
	assert.EqualValues(t, math.MinInt64, safeSubClip(math.MinInt64, math.MaxInt64))
	assert.EqualValues(t, math.MaxInt64, safeSubClip(math.MaxInt64, -10))
}

func TestUpdatesForNewValidatorSet(t *testing.T) {
	t.Parallel()

	v1 := &ValidatorMetadata{Address: types.Address{0x1}, VotingPower: 100}
	v2 := &ValidatorMetadata{Address: types.Address{0x2}, VotingPower: 100}
	accountSet := []*ValidatorMetadata{v1, v2}
	valSet, err := NewValidatorSet(accountSet, hclog.NewNullLogger())
	require.NoError(t, err)

	err = valSet.IncrementProposerPriority(1)
	require.NoError(t, err)
	verifyValidatorSet(t, valSet)
}

func verifyValidatorSet(t *testing.T, valSet *validatorSet) {
	t.Helper()
	// verify that the capacity and length of validators is the same
	assert.Equal(t, len(valSet.Accounts()), cap(valSet.validators))
	// verify that the set's total voting power has been updated
	tvp := valSet.totalVotingPower
	err := valSet.updateTotalVotingPower()
	require.NoError(t, err)
	expectedTvp, err := valSet.TotalVotingPower()
	require.NoError(t, err)
	assert.Equal(t, expectedTvp, tvp,
		"expected TVP %d. Got %d, valSet=%s", expectedTvp, tvp, valSet)
	// verify that validator priorities are centered
	valsCount := int64(len(valSet.validators))
	tpp := valSetTotalProposerPriority(valSet)
	assert.True(t, tpp < valsCount && tpp > -valsCount,
		"expected total priority in (-%d, %d). Got %d", valsCount, valsCount, tpp)
	// verify that priorities are scaled
	dist := computeMaxMinPriorityDiff(valSet)
	assert.True(t, dist <= priorityWindowSizeFactor*tvp,
		"expected priority distance < %d. Got %d", priorityWindowSizeFactor*tvp, dist)
}

func valSetTotalProposerPriority(valSet *validatorSet) int64 {
	sum := int64(0)
	for _, val := range valSet.validators {
		// mind overflow
		sum = safeAddClip(sum, val.ProposerPriority)
	}

	return sum
}
