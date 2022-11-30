package polybft

import (
	"bytes"
	"math"
	"testing"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposerCalculator_SetIndex(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"}, []uint64{10, 100, 1, 50, 30})
	metadata := validators.getPublicIdentities()

	vs, err := validators.toValidatorSet()
	require.NoError(t, err)

	pc, err := NewProposerCalculator(metadata, vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, metadata[i].Address, v.Address)
	}

	proposer, err := pc.CalcProposer(0)
	require.NoError(t, err)
	assert.Equal(t, proposer, metadata[1].Address)
	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, metadata[i].Address, v.Address)
	}
}

func TestProposerCalculator_CalculateProposer(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E"}, []uint64{1, 2, 3, 4, 5})
	metadata := validators.getPublicIdentities()

	vs, err := validators.toValidatorSet()
	require.NoError(t, err)

	pc, err := NewProposerCalculator(metadata, vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	assert.Equal(t, int64(15), vs.totalVotingPower)

	currProposerAddress, err := pc.CalcProposer(0)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, currProposerAddress)

	proposerAddressR1, err := pc.CalcProposer(1)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR1)

	proposerAddressR2, err := pc.CalcProposer(2)
	require.NoError(t, err)
	assert.Equal(t, metadata[2].Address, proposerAddressR2)

	proposerAddressR3, err := pc.CalcProposer(3)
	require.NoError(t, err)
	assert.Equal(t, metadata[1].Address, proposerAddressR3)

	proposerAddressR4, err := pc.CalcProposer(4)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, proposerAddressR4)

	proposerAddressR5, err := pc.CalcProposer(5)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR5)

	proposerAddressR6, err := pc.CalcProposer(6)
	require.NoError(t, err)
	assert.Equal(t, metadata[0].Address, proposerAddressR6)
}

func TestProposerCalculator_SamePriority(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(5)
	require.NoError(t, err)

	// at some point priorities will be the same and bytes address will be compared
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

	pc, err := NewProposerCalculator(vs.Accounts(), vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	proposerR0, err := pc.CalcProposer(0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerR0)

	proposerR1, err := pc.CalcProposer(1)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerR1)

	proposerR2, err := pc.CalcProposer(2)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerR2)
}

func TestProposerCalculator_ProposerSelection1(t *testing.T) {
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

	pc, err := NewProposerCalculator(vset.Accounts(), vset.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	var proposers = make([]types.Address, numberOfIteration)

	for i := uint64(0); i < numberOfIteration; i++ {
		proposers[i], err = pc.CalcProposer(i)
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
func TestProposerCalculator_IncrementProposerPriorityPositiveTimes(t *testing.T) {
	t.Parallel()

	validators := newTestValidatorsWithAliases([]string{"A", "B", "C"}, []uint64{1000, 300, 330})
	metadata := validators.getPublicIdentities()

	vs, err := validators.toValidatorSet()
	require.NoError(t, err)

	pc, err := NewProposerCalculator(metadata, vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	proposerAddressR0, err := pc.CalcProposer(0)
	assert.Equal(t, metadata[0].Address, proposerAddressR0)

	// priority must be > 0
	err = pc.IncrementProposerPriority(0)
	require.Error(t, err)

	proposerAddressR1, err := pc.CalcProposer(1)
	require.NoError(t, err)

	assert.Equal(t, metadata[2].Address, proposerAddressR1)
}

func TestProposerCalculator_IncrementProposerPrioritySameVotingPower(t *testing.T) {
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

	pc, err := NewProposerCalculator(vs.Accounts(), vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	// when voting power is the same order is by address
	currProposerAddress, err := pc.CalcProposer(0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, currProposerAddress)

	proposerAddresR1, err := pc.CalcProposer(1)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerAddresR1)

	proposerAddressR2, err := pc.CalcProposer(2)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerAddressR2)

	proposerAddressR3, err := pc.CalcProposer(3)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerAddressR3)

	proposerAddressR4, err := pc.CalcProposer(4)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerAddressR4)
}

func TestProposerCalculator_AveragingInIncrementProposerPriorityWithVotingPower(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	// Other than TestAveragingInIncrementProposerPriority this is a more complete test showing
	// how each ProposerPriority changes in relation to the validator's voting power respectively.
	// average is zero in each round:
	vp0 := int64(10)
	vp1 := int64(1)
	vp2 := int64(1)
	total := vp0 + vp1 + vp2
	avg := (vp0 + vp1 + vp2 - total) / 3
	valz := []*ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: uint64(vp0),
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: uint64(vp1),
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: uint64(vp2),
		},
	}

	vals, err := NewValidatorSet(valz, hclog.NewNullLogger())
	assert.NoError(t, err)

	tcs := []struct {
		wantProposerPrioritys []int64
		times                 uint64
		wantProposerIndex     int64
	}{

		0: {
			[]int64{
				// Acumm+VotingPower-Avg:
				0 + vp0 - total - avg, // mostest will be subtracted by total voting power (12)
				0 + vp1,
				0 + vp2},
			1,
			0,
		},
		1: {
			[]int64{
				(0 + vp0 - total) + vp0 - total - avg, // this will be mostest on 2nd iter, too
				(0 + vp1) + vp1,
				(0 + vp2) + vp2},
			2,
			0,
		}, // increment twice -> expect average to be subtracted twice
		2: {
			[]int64{
				0 + 3*(vp0-total) - avg, // still mostest
				0 + 3*vp1,
				0 + 3*vp2},
			3,
			0,
		},
		3: {
			[]int64{
				0 + 4*(vp0-total), // still mostest
				0 + 4*vp1,
				0 + 4*vp2},
			4,
			0,
		},
		4: {
			[]int64{
				0 + 4*(vp0-total) + vp0, // 4 iters was mostest
				0 + 5*vp1 - total,       // now this val is mostest for the 1st time (hence -12==totalVotingPower)
				0 + 5*vp2},
			5,
			1,
		},
		5: {
			[]int64{
				0 + 6*vp0 - 5*total, // mostest again
				0 + 6*vp1 - total,   // mostest once up to here
				0 + 6*vp2},
			6,
			0,
		},
		6: {
			[]int64{
				0 + 7*vp0 - 6*total, // in 7 iters this val is mostest 6 times
				0 + 7*vp1 - total,   // in 7 iters this val is mostest 1 time
				0 + 7*vp2},
			7,
			0,
		},
		7: {
			[]int64{
				0 + 8*vp0 - 7*total, // mostest again
				0 + 8*vp1 - total,
				0 + 8*vp2},
			8,
			0,
		},
		8: {
			[]int64{
				0 + 9*vp0 - 7*total,
				0 + 9*vp1 - total,
				0 + 9*vp2 - total}, // mostest
			9,
			2,
		},
		9: {
			[]int64{
				0 + 10*vp0 - 8*total, // after 10 iters this is mostest again
				0 + 10*vp1 - total,   // after 6 iters this val is "mostest" once and not in between
				0 + 10*vp2 - total},  // in between 10 iters this val is "mostest" once
			10,
			0,
		},
		10: {
			[]int64{
				0 + 11*vp0 - 9*total,
				0 + 11*vp1 - total,  // after 6 iters this val is "mostest" once and not in between
				0 + 11*vp2 - total}, // after 10 iters this val is "mostest" once
			11,
			0,
		},
	}

	for i, tc := range tcs {
		pc, err := NewProposerCalculator(valz, vals.totalVotingPower, hclog.NewNullLogger())
		assert.NoError(t, err)

		err = pc.IncrementProposerPriority(tc.times)
		assert.NoError(t, err)

		proposer := pc.proposer
		if proposer == nil {
			proposer, err = pc.getValWithMostPriority()
			require.NoError(t, err)
		}

		assert.Equal(t, pc.validators[tc.wantProposerIndex].Metadata.Address, proposer.Metadata.Address,
			"test case: %v",
			i)

		for valIdx, val := range pc.validators {
			assert.Equal(t,
				tc.wantProposerPrioritys[valIdx],
				val.ProposerPriority,
				"test case: %v, validator: %v",
				i,
				valIdx)
		}
	}
}

func TestProposerCalculator_TotalVotingPowerErrorOnOverflow(t *testing.T) {
	t.Parallel()

	// NewValidatorSet calls IncrementProposerPriority which calls TotalVotingPower()
	// which should panic on overflows:
	_, err := NewValidatorSet([]*ValidatorMetadata{
		{Address: types.Address{0x1}, VotingPower: math.MaxInt64},
		{Address: types.Address{0x2}, VotingPower: math.MaxInt64},
		{Address: types.Address{0x3}, VotingPower: math.MaxInt64},
	}, hclog.NewNullLogger())
	require.Error(t, err)
}

func TestProposerCalculator_UpdatesForNewValidatorSet(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(2)
	require.NoError(t, err)

	v1 := &ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: 100}
	v2 := &ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: 100}

	accountSet := []*ValidatorMetadata{v1, v2}
	vs, err := NewValidatorSet(accountSet, hclog.NewNullLogger())
	require.NoError(t, err)

	pc, err := NewProposerCalculator(vs.Accounts(), vs.totalVotingPower, hclog.NewNullLogger())
	require.NoError(t, err)

	_, err = pc.CalcProposer(1)
	require.NoError(t, err)

	// verify that the capacity and length of validators is the same
	assert.Equal(t, len(vs.Accounts()), cap(pc.validators))
	// verify that validator priorities are centered
	valsCount := int64(len(pc.validators))

	sum := int64(0)
	for _, val := range pc.validators {
		// mind overflow
		sum = safeAddClip(sum, val.ProposerPriority)
	}

	assert.True(t, sum < valsCount && sum > -valsCount,
		"expected total priority in (-%d, %d). Got %d", valsCount, valsCount, sum)
	// verify that priorities are scaled
	dist := computeMaxMinPriorityDiff(pc.validators)
	assert.True(t, dist <= priorityWindowSizeFactor*vs.totalVotingPower,
		"expected priority distance < %d. Got %d", priorityWindowSizeFactor*vs.totalVotingPower, dist)
}
