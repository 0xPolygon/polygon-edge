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

	snapshot := NewProposerSnapshot(1, metadata)
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, metadata[i].Address, v.Address)
	}

	proposer, err := pc.CalcProposer(0, 1)
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

	snapshot := NewProposerSnapshot(0, metadata)
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	assert.Equal(t, int64(15), vs.totalVotingPower)

	currProposerAddress, err := pc.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, currProposerAddress)

	proposerAddressR1, err := pc.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR1)

	proposerAddressR2, err := pc.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[2].Address, proposerAddressR2)

	proposerAddressR3, err := pc.CalcProposer(3, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[1].Address, proposerAddressR3)

	proposerAddressR4, err := pc.CalcProposer(4, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, proposerAddressR4)

	proposerAddressR5, err := pc.CalcProposer(5, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR5)

	proposerAddressR6, err := pc.CalcProposer(6, 0)
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

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	proposerR0, err := pc.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerR0)

	proposerR1, err := pc.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerR1)

	proposerR2, err := pc.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerR2)

	proposerR2, err = pc.CalcProposer(2, 0) // call again same round
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

	snapshot := NewProposerSnapshot(4, vset.Accounts())
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	var proposers = make([]types.Address, numberOfIteration)

	for i := uint64(0); i < numberOfIteration; i++ {
		proposers[i], err = pc.CalcProposer(i, 4)
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

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	// when voting power is the same order is by address
	currProposerAddress, err := pc.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, currProposerAddress)

	proposerAddresR1, err := pc.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerAddresR1)

	proposerAddressR2, err := pc.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerAddressR2)

	proposerAddressR3, err := pc.CalcProposer(3, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerAddressR3)

	proposerAddressR4, err := pc.CalcProposer(4, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerAddressR4)
}

// func TestProposerCalculator_AveragingInIncrementProposerPriorityWithVotingPower(t *testing.T) {
// 	t.Parallel()

// 	keys, err := bls.CreateRandomBlsKeys(3)
// 	require.NoError(t, err)

// 	// Other than TestAveragingInIncrementProposerPriority this is a more complete test showing
// 	// how each ProposerPriority changes in relation to the validator's voting power respectively.
// 	// average is zero in each round:
// 	vp0 := int64(10)
// 	vp1 := int64(1)
// 	vp2 := int64(1)
// 	total := vp0 + vp1 + vp2
// 	avg := (vp0 + vp1 + vp2 - total) / 3
// 	valz := []*ValidatorMetadata{
// 		{
// 			BlsKey:      keys[0].PublicKey(),
// 			Address:     types.Address{0x1},
// 			VotingPower: uint64(vp0),
// 		},
// 		{
// 			BlsKey:      keys[1].PublicKey(),
// 			Address:     types.Address{0x2},
// 			VotingPower: uint64(vp1),
// 		},
// 		{
// 			BlsKey:      keys[2].PublicKey(),
// 			Address:     types.Address{0x3},
// 			VotingPower: uint64(vp2),
// 		},
// 	}

// 	tcs := []struct {
// 		wantProposerPrioritys []int64
// 		times                 uint64
// 		wantProposerIndex     int64
// 	}{

// 		0: {
// 			[]int64{
// 				// Acumm+VotingPower-Avg:
// 				0 + vp0 - total - avg, // mostest will be subtracted by total voting power (12)
// 				0 + vp1,
// 				0 + vp2},
// 			1,
// 			0,
// 		},
// 		1: {
// 			[]int64{
// 				(0 + vp0 - total) + vp0 - total - avg, // this will be mostest on 2nd iter, too
// 				(0 + vp1) + vp1,
// 				(0 + vp2) + vp2},
// 			2,
// 			0,
// 		}, // increment twice -> expect average to be subtracted twice
// 		2: {
// 			[]int64{
// 				0 + 3*(vp0-total) - avg, // still mostest
// 				0 + 3*vp1,
// 				0 + 3*vp2},
// 			3,
// 			0,
// 		},
// 		3: {
// 			[]int64{
// 				0 + 4*(vp0-total), // still mostest
// 				0 + 4*vp1,
// 				0 + 4*vp2},
// 			4,
// 			0,
// 		},
// 		4: {
// 			[]int64{
// 				0 + 4*(vp0-total) + vp0, // 4 iters was mostest
// 				0 + 5*vp1 - total,       // now this val is mostest for the 1st time (hence -12==totalVotingPower)
// 				0 + 5*vp2},
// 			5,
// 			1,
// 		},
// 		5: {
// 			[]int64{
// 				0 + 6*vp0 - 5*total, // mostest again
// 				0 + 6*vp1 - total,   // mostest once up to here
// 				0 + 6*vp2},
// 			6,
// 			0,
// 		},
// 		6: {
// 			[]int64{
// 				0 + 7*vp0 - 6*total, // in 7 iters this val is mostest 6 times
// 				0 + 7*vp1 - total,   // in 7 iters this val is mostest 1 time
// 				0 + 7*vp2},
// 			7,
// 			0,
// 		},
// 		7: {
// 			[]int64{
// 				0 + 8*vp0 - 7*total, // mostest again
// 				0 + 8*vp1 - total,
// 				0 + 8*vp2},
// 			8,
// 			0,
// 		},
// 		8: {
// 			[]int64{
// 				0 + 9*vp0 - 7*total,
// 				0 + 9*vp1 - total,
// 				0 + 9*vp2 - total}, // mostest
// 			9,
// 			2,
// 		},
// 		9: {
// 			[]int64{
// 				0 + 10*vp0 - 8*total, // after 10 iters this is mostest again
// 				0 + 10*vp1 - total,   // after 6 iters this val is "mostest" once and not in between
// 				0 + 10*vp2 - total},  // in between 10 iters this val is "mostest" once
// 			10,
// 			0,
// 		},
// 		10: {
// 			[]int64{
// 				0 + 11*vp0 - 9*total,
// 				0 + 11*vp1 - total,  // after 6 iters this val is "mostest" once and not in between
// 				0 + 11*vp2 - total}, // after 10 iters this val is "mostest" once
// 			11,
// 			0,
// 		},
// 	}

// 	for i, tc := range tcs {
// 		snap := NewProposerSnapshot(1, valz)
// 		pc := NewProposerCalculatorFromSnapshot(snap, hclog.NewNullLogger())

// 		_, err := incrementProposerPriorityNTimes(snap, tc.times)
// 		require.NoError(t, err)

// 		address, _ := pc.GetLatestProposer(tc.times-1, 1)

// 		assert.Equal(t, snap.Validators[tc.wantProposerIndex].Metadata.Address, address,
// 			"test case: %v",
// 			i)

// 		for valIdx, val := range snap.Validators {
// 			assert.Equal(t,
// 				tc.wantProposerPrioritys[valIdx],
// 				val.ProposerPriority,
// 				"test case: %v, validator: %v",
// 				i,
// 				valIdx)
// 		}
// 	}
// }

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

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	pc := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

	_, err = pc.CalcProposer(1, 0)
	require.NoError(t, err)

	// verify that the capacity and length of validators is the same
	assert.Equal(t, len(vs.Accounts()), cap(snapshot.Validators))
	// verify that validator priorities are centered
	valsCount := int64(len(snapshot.Validators))

	sum := int64(0)
	for _, val := range snapshot.Validators {
		// mind overflow
		sum = safeAddClip(sum, val.ProposerPriority)
	}

	assert.True(t, sum < valsCount && sum > -valsCount,
		"expected total priority in (-%d, %d). Got %d", valsCount, valsCount, sum)
	// verify that priorities are scaled
	dist := computeMaxMinPriorityDiff(snapshot.Validators)
	assert.True(t, dist <= priorityWindowSizeFactor*vs.totalVotingPower,
		"expected priority distance < %d. Got %d", priorityWindowSizeFactor*vs.totalVotingPower, dist)
}

// func TestProposerCalculator_GetLatestProposer(t *testing.T) {
// 	const (
// 		bestIdx = 5
// 		count   = 10
// 	)

// 	t.Parallel()

// 	validatorSet := newTestValidators(count).getPublicIdentities()
// 	snapshot := NewProposerSnapshot(0, validatorSet)
// 	proposerCalculator := NewProposerCalculatorFromSnapshot(snapshot, hclog.NewNullLogger())

// 	snapshot.Validators[bestIdx].ProposerPriority = 1000000

// 	// not set
// 	_, exists := proposerCalculator.GetLatestProposer(0, 0)
// 	assert.False(t, exists)

// 	_, err := proposerCalculator.CalcProposer(0, 0)
// 	require.NoError(t, err)

// 	// wrong round
// 	_, exists = proposerCalculator.GetLatestProposer(1, 0)
// 	assert.False(t, exists)

// 	// wrong height
// 	_, exists = proposerCalculator.GetLatestProposer(0, 1)
// 	assert.False(t, exists)

// 	// ok
// 	address, exists := proposerCalculator.GetLatestProposer(0, 0)
// 	assert.True(t, exists)

// 	assert.Equal(t, validatorSet[bestIdx].Address, address)
// }

func TestProposerCalculator_UpdateValidators(t *testing.T) {
	const rounds = 7

	desiredPriorities := []int64{-300, -300, 0}

	keys, err := bls.CreateRandomBlsKeys(8)
	require.NoError(t, err)

	v1 := &ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: 100}
	v2 := &ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: 100}
	v3 := &ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: 100}
	v4 := &ValidatorMetadata{Address: types.Address{0x4}, BlsKey: keys[3].PublicKey(), VotingPower: 100}
	v5 := &ValidatorMetadata{Address: types.Address{0x5}, BlsKey: keys[4].PublicKey(), VotingPower: 100}

	accountSet := []*ValidatorMetadata{v1, v2, v3, v4, v5}
	vs, err := NewValidatorSet(accountSet, hclog.NewNullLogger())
	require.NoError(t, err)

	snap := NewProposerSnapshot(0, vs.Accounts())

	_, err = incrementProposerPriorityNTimes(snap, 7)
	require.NoError(t, err)

	// update
	u1 := &ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[5].PublicKey(), VotingPower: 10}
	u2 := &ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[6].PublicKey(), VotingPower: 550}

	// new
	u3 := &ValidatorMetadata{Address: types.Address{0x9}, BlsKey: keys[7].PublicKey(), VotingPower: 200}

	newAccountSet := []*ValidatorMetadata{u1, u2, u3}

	updateValidators(snap, newAccountSet)

	for i, v := range snap.Validators {
		assert.Equal(t, desiredPriorities[i], v.ProposerPriority)
	}
}
