package polybft

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposerCalculator_SetIndex(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"}, []uint64{10, 100, 1, 50, 30})
	metadata := validators.GetPublicIdentities()

	vs := validators.ToValidatorSet()

	snapshot := NewProposerSnapshot(1, metadata)

	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, metadata[i].Address, v.Address)
	}

	proposer, err := snapshot.CalcProposer(0, 1)
	require.NoError(t, err)
	assert.Equal(t, proposer, metadata[1].Address)
	// validate no changes to validator set positions
	for i, v := range vs.Accounts() {
		assert.Equal(t, metadata[i].Address, v.Address)
	}
}

func TestProposerCalculator_RegularFlow(t *testing.T) {
	t.Parallel()

	validators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E"}, []uint64{1, 2, 3, 4, 5})
	metadata := validators.GetPublicIdentities()

	snapshot := NewProposerSnapshot(0, metadata)

	currProposerAddress, err := snapshot.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, currProposerAddress)

	proposerAddressR1, err := snapshot.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR1)

	proposerAddressR2, err := snapshot.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[2].Address, proposerAddressR2)

	proposerAddressR3, err := snapshot.CalcProposer(3, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[1].Address, proposerAddressR3)

	proposerAddressR4, err := snapshot.CalcProposer(4, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[4].Address, proposerAddressR4)

	proposerAddressR5, err := snapshot.CalcProposer(5, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[3].Address, proposerAddressR5)

	proposerAddressR6, err := snapshot.CalcProposer(6, 0)
	require.NoError(t, err)
	assert.Equal(t, metadata[0].Address, proposerAddressR6)
}

func TestProposerCalculator_SamePriority(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(5)
	require.NoError(t, err)

	// at some point priorities will be the same and bytes address will be compared
	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: big.NewInt(1),
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: big.NewInt(2),
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: big.NewInt(3),
		},
	}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())

	proposerR0, err := snapshot.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerR0)

	proposerR1, err := snapshot.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerR1)

	proposerR2, err := snapshot.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerR2)

	proposerR2, err = snapshot.CalcProposer(2, 0) // call again same round
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerR2)
}

func TestProposerCalculator_InversePriorityOrderWithExpectedListOfSelection(t *testing.T) {
	t.Parallel()

	const numberOfIteration = 99

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	// priorities are from high to low vp in validator set
	vset := validator.NewValidatorSet([]*validator.ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: big.NewInt(1000),
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: big.NewInt(300),
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: big.NewInt(330),
		},
	}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(4, vset.Accounts())

	var proposers = make([]types.Address, numberOfIteration)

	for i := uint64(0); i < numberOfIteration; i++ {
		proposers[i], err = snapshot.CalcProposer(i, 4)
		require.NoError(t, err)
	}

	// list of addresses in order that should be selected
	expectedValidatorAddresses := []types.Address{
		{0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1},
		{0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1},
		{0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3},
		{0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x1}, {0x3}, {0x2}, {0x1}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2},
		{0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1}, {0x1}, {0x3}, {0x1}, {0x2}, {0x1},
		{0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1}, {0x3}, {0x1}, {0x1}, {0x2}, {0x1},
		{0x3}, {0x1}, {0x1},
	}

	for i, p := range proposers {
		assert.True(t, bytes.Equal(expectedValidatorAddresses[i].Bytes(), p.Bytes()))
	}
}

func TestProposerCalculator_IncrementProposerPrioritySameVotingPower(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: big.NewInt(1),
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: big.NewInt(1),
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: big.NewInt(1),
		},
	}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())

	// when voting power is the same order is by address
	currProposerAddress, err := snapshot.CalcProposer(0, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, currProposerAddress)

	proposerAddresR1, err := snapshot.CalcProposer(1, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x2}, proposerAddresR1)

	proposerAddressR2, err := snapshot.CalcProposer(2, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x3}, proposerAddressR2)

	proposerAddressR3, err := snapshot.CalcProposer(3, 0)
	require.NoError(t, err)
	assert.Equal(t, types.Address{0x1}, proposerAddressR3)

	proposerAddressR4, err := snapshot.CalcProposer(4, 0)
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
	valz := []*validator.ValidatorMetadata{
		{
			BlsKey:      keys[0].PublicKey(),
			Address:     types.Address{0x1},
			VotingPower: big.NewInt(vp0),
		},
		{
			BlsKey:      keys[1].PublicKey(),
			Address:     types.Address{0x2},
			VotingPower: big.NewInt(vp1),
		},
		{
			BlsKey:      keys[2].PublicKey(),
			Address:     types.Address{0x3},
			VotingPower: big.NewInt(vp2),
		},
	}

	tcs := []struct {
		wantProposerPriority []int64
		times                uint64
		wantProposerIndex    int64
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
		snap := NewProposerSnapshot(1, valz)

		_, err := incrementProposerPriorityNTimes(snap, tc.times)
		require.NoError(t, err)

		address, _ := snap.GetLatestProposer(tc.times-1, 1)

		assert.Equal(t, snap.Validators[tc.wantProposerIndex].Metadata.Address, address,
			"test case: %v",
			i)

		for valIdx, val := range snap.Validators {
			assert.Equal(t,
				tc.wantProposerPriority[valIdx],
				val.ProposerPriority.Int64(),
				"test case: %v, validator: %v",
				i,
				valIdx)
		}
	}
}

func TestProposerCalculator_UpdatesForNewValidatorSet(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(2)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(100)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(100)}

	accountSet := []*validator.ValidatorMetadata{v1, v2}
	vs := validator.NewValidatorSet(accountSet, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())

	_, err = snapshot.CalcProposer(1, 0)
	require.NoError(t, err)

	// verify that the capacity and length of validators is the same
	assert.Equal(t, len(vs.Accounts()), cap(snapshot.Validators))
	// verify that validator priorities are centered
	valsCount := int64(len(snapshot.Validators))

	sum := big.NewInt(0)
	for _, val := range snapshot.Validators {
		// mind overflow
		sum = new(big.Int).Add(sum, val.ProposerPriority)
	}

	assert.True(t, sum.Cmp(big.NewInt(valsCount)) < 0 && sum.Cmp(big.NewInt(-valsCount)) > 0,
		"expected total priority in (-%d, %d). Got %d", valsCount, valsCount, sum)

	// verify that priorities are scaled
	diff := computeMaxMinPriorityDiff(snapshot.Validators)
	totalVotingPower := vs.TotalVotingPower()
	diffMax := new(big.Int).Mul(priorityWindowSizeFactor, &totalVotingPower)
	assert.True(t, diff.Cmp(diffMax) <= 0, "expected priority distance < %d. Got %d", diffMax, diff)
}

func TestProposerCalculator_GetLatestProposer(t *testing.T) {
	t.Parallel()

	const (
		bestIdx = 5
		count   = 10
	)

	validatorSet := validator.NewTestValidators(t, count).GetPublicIdentities()
	snapshot := NewProposerSnapshot(0, validatorSet)
	snapshot.Validators[bestIdx].ProposerPriority = big.NewInt(1000000)

	// not set
	_, err := snapshot.GetLatestProposer(0, 0)
	assert.Error(t, err)

	_, err = snapshot.CalcProposer(0, 0)
	assert.NoError(t, err)

	// wrong round
	_, err = snapshot.GetLatestProposer(1, 0)
	assert.Error(t, err)

	// wrong height
	_, err = snapshot.GetLatestProposer(0, 1)
	assert.Error(t, err)

	// ok
	address, err := snapshot.GetLatestProposer(0, 0)
	assert.NoError(t, err)

	proposerAddress := validatorSet[bestIdx].Address
	assert.Equal(t, proposerAddress, address)
}

func TestProposerCalculator_UpdateValidatorsSameVpUpdatedAndNewAdded(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(8)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(100)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(100)}
	v3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(100)}
	v4 := &validator.ValidatorMetadata{Address: types.Address{0x4}, BlsKey: keys[3].PublicKey(), VotingPower: big.NewInt(100)}
	v5 := &validator.ValidatorMetadata{Address: types.Address{0x5}, BlsKey: keys[4].PublicKey(), VotingPower: big.NewInt(100)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2, v3, v4, v5}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())

	// iterate one cycle should bring back priority to 0
	_, err = incrementProposerPriorityNTimes(snapshot, 5)
	require.NoError(t, err)

	for _, v := range snapshot.Validators {
		assert.True(t, v.ProposerPriority.Cmp(big.NewInt(0)) == 0)
	}

	// updated old validators
	u1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(10)}
	u2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(10)}
	// added new validator
	a1 := &validator.ValidatorMetadata{Address: types.Address{0x9}, BlsKey: keys[7].PublicKey(), VotingPower: big.NewInt(100)}

	newAccountSet := []*validator.ValidatorMetadata{u1, u2, a1}

	require.NoError(t, updateValidators(snapshot, newAccountSet))
	assert.Equal(t, 3, len(snapshot.Validators))

	// removedVp := sum(v3, v4, v5) = 300
	// newVp := sum(u1, u2, a1) = 120
	// sum(removedVp, newVp) = 420; priority(a1) = -1.125*420 = -472
	// scale: difMax = 2 * 120; diff(-475, 0); ratio ~ 2
	// priority(a1) = -472/2 = 236; u1 = 0, u2 = 0
	// center: avg = 236/3 = 79; priority(a1)= 236 - 79

	// check voting power after update
	assert.Equal(t, big.NewInt(10), snapshot.Validators[0].Metadata.VotingPower)
	assert.Equal(t, big.NewInt(10), snapshot.Validators[1].Metadata.VotingPower)
	assert.Equal(t, big.NewInt(100), snapshot.Validators[2].Metadata.VotingPower)
	// newly added validator
	assert.Equal(t, big.NewInt(100), snapshot.Validators[2].Metadata.VotingPower)
	assert.Equal(t, types.Address{0x9}, snapshot.Validators[2].Metadata.Address)
	assert.Equal(t, big.NewInt(-157), snapshot.Validators[2].ProposerPriority) // a1
	// check priority
	assert.Equal(t, big.NewInt(79), snapshot.Validators[0].ProposerPriority) // u1
	assert.Equal(t, big.NewInt(79), snapshot.Validators[1].ProposerPriority) // u2

	_, err = incrementProposerPriorityNTimes(snapshot, 1)
	require.NoError(t, err)

	// 79 + 10 - (100+10+10)
	assert.Equal(t, big.NewInt(-31), snapshot.Validators[0].ProposerPriority)
	// 79 + 10
	assert.Equal(t, big.NewInt(89), snapshot.Validators[1].ProposerPriority)
	// -157+100
	assert.Equal(t, big.NewInt(-57), snapshot.Validators[2].ProposerPriority)
}

func TestProposerCalculator_UpdateValidators(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(4)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(10)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(20)}
	v3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(30)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2, v3}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	require.Equal(t, big.NewInt(60), snapshot.GetTotalVotingPower())
	// 	init priority must be 0
	require.Zero(t, snapshot.Validators[0].ProposerPriority.Int64())
	require.Zero(t, snapshot.Validators[1].ProposerPriority.Int64())
	require.Zero(t, snapshot.Validators[2].ProposerPriority.Int64())
	// vp must be initialized
	require.Equal(t, big.NewInt(10), snapshot.Validators[0].Metadata.VotingPower)
	require.Equal(t, big.NewInt(20), snapshot.Validators[1].Metadata.VotingPower)
	require.Equal(t, big.NewInt(30), snapshot.Validators[2].Metadata.VotingPower)

	// increment once
	_, err = incrementProposerPriorityNTimes(snapshot, 1)
	require.NoError(t, err)

	// updated
	u1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(100)}
	u2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(200)}
	u3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(300)}
	// added
	a1 := &validator.ValidatorMetadata{Address: types.Address{0x4}, BlsKey: keys[3].PublicKey(), VotingPower: big.NewInt(400)}

	require.NoError(t, updateValidators(snapshot, []*validator.ValidatorMetadata{u1, u2, u3, a1}))

	require.Equal(t, 4, len(snapshot.Validators))
	// priorities are from previous iteration
	require.Equal(t, big.NewInt(292), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(302), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(252), snapshot.Validators[2].ProposerPriority)
	// new added a1
	require.Equal(t, types.Address{0x4}, snapshot.Validators[3].Metadata.Address)
	require.Equal(t, big.NewInt(-843), snapshot.Validators[3].ProposerPriority)
	// total vp is updated
	require.Equal(t, big.NewInt(1000), snapshot.GetTotalVotingPower())
}

func TestProposerCalculator_ScaleAfterDelete(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(10)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(10)}
	v3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(80000)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2, v3}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	assert.Equal(t, big.NewInt(80020), snapshot.GetTotalVotingPower())

	_, err = incrementProposerPriorityNTimes(snapshot, 1)
	require.NoError(t, err)

	// priorities are from previous iteration
	require.Equal(t, big.NewInt(10), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(10), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(-20), snapshot.Validators[2].ProposerPriority)

	// another increment
	proposer, err := incrementProposerPriorityNTimes(snapshot, 4000)
	require.NoError(t, err)
	// priorities are from previous iteration
	assert.Equal(t, types.Address{0x3}, proposer.Metadata.Address)

	// 	reduce validator voting power from 8k to 1
	u1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(10)}
	u2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(10)}

	require.Equal(t, big.NewInt(-40010), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(40010), snapshot.Validators[1].ProposerPriority)

	require.NoError(t, updateValidators(snapshot, []*validator.ValidatorMetadata{u1, u2}))

	// maxdiff = 2*tvp = 40
	// diff(min,max) (-40010, 40010) = 80020
	// ratio := (diff + diffMax - 1) / diffMax; (80020 + 20 - 1)/20 = 2001
	// priority = priority / ratio; u1 = -40010 / 4001 ~ -19; u2 = 40010 / 4001 ~ 19
	require.Equal(t, big.NewInt(-19), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(19), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(20), snapshot.GetTotalVotingPower())
}

func TestProposerCalculator_ShiftAfterUpdate(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(50)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(80)}
	v3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(100000)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2, v3}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	assert.Equal(t, big.NewInt(100130), snapshot.GetTotalVotingPower())

	_, err = incrementProposerPriorityNTimes(snapshot, 4000)
	require.NoError(t, err)

	// updates
	u1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(5)}
	u2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(8)}

	require.NoError(t, updateValidators(snapshot, []*validator.ValidatorMetadata{u1, u2}))

	// maxdiff = 2*tvp = 26
	// diff(min,max) (-260, 19610) = 19870
	// ratio := (diff + diffMax - 1) / diffMax; (19870 + 26 - 1)/26 =765
	// scale priority = priority / ratio; p1 = 0; p2 = 25
	// shift with avg=(25+0)/2=12; p = priority - avg; u1 = -12; u2= 13
	require.Equal(t, big.NewInt(-12), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(13), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(13), snapshot.GetTotalVotingPower())
}

func TestProposerCalculator_UpdateValidatorSet(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(1)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(8)}
	v3 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(15)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2, v3}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	assert.Equal(t, big.NewInt(24), snapshot.GetTotalVotingPower())

	_, err = incrementProposerPriorityNTimes(snapshot, 2)
	require.NoError(t, err)

	// modified validator
	u1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(5)}
	// added validator
	a1 := &validator.ValidatorMetadata{Address: types.Address{0x4}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(8)}

	require.NoError(t, updateValidators(snapshot, []*validator.ValidatorMetadata{u1, a1}))
	// expecting 2 validators with updated voting power and total voting power
	require.Equal(t, 2, len(snapshot.Validators))
	require.Equal(t, types.Address{0x1}, snapshot.Validators[0].Metadata.Address)
	require.Equal(t, big.NewInt(5), snapshot.Validators[0].Metadata.VotingPower)
	require.Equal(t, big.NewInt(11), snapshot.Validators[0].ProposerPriority)

	require.Equal(t, types.Address{0x4}, snapshot.Validators[1].Metadata.Address)
	require.Equal(t, big.NewInt(8), snapshot.Validators[1].Metadata.VotingPower)
	require.Equal(t, big.NewInt(-10), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(13), snapshot.GetTotalVotingPower())
}

func TestProposerCalculator_AddValidator(t *testing.T) {
	t.Parallel()

	keys, err := bls.CreateRandomBlsKeys(3)
	require.NoError(t, err)

	v1 := &validator.ValidatorMetadata{Address: types.Address{0x1}, BlsKey: keys[0].PublicKey(), VotingPower: big.NewInt(3)}
	v2 := &validator.ValidatorMetadata{Address: types.Address{0x2}, BlsKey: keys[1].PublicKey(), VotingPower: big.NewInt(1)}

	vs := validator.NewValidatorSet([]*validator.ValidatorMetadata{v1, v2}, hclog.NewNullLogger())

	snapshot := NewProposerSnapshot(0, vs.Accounts())
	assert.Equal(t, big.NewInt(4), snapshot.GetTotalVotingPower())
	proposer, err := incrementProposerPriorityNTimes(snapshot, 1)
	require.NoError(t, err)
	require.Equal(t, types.Address{0x1}, proposer.Metadata.Address)
	require.Equal(t, big.NewInt(-1), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(1), snapshot.Validators[1].ProposerPriority)

	_, err = incrementProposerPriorityNTimes(snapshot, 1)
	require.NoError(t, err)

	require.Equal(t, big.NewInt(-2), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(2), snapshot.Validators[1].ProposerPriority)

	a1 := &validator.ValidatorMetadata{Address: types.Address{0x3}, BlsKey: keys[2].PublicKey(), VotingPower: big.NewInt(8)}

	require.NoError(t, updateValidators(snapshot, []*validator.ValidatorMetadata{v1, v2, a1}))

	// updated vp: 8+3+1 = 12
	// added validator priority = -1.125*8 ~ -13
	// scaling: max(-13, 3) = 16 < 2* 12; no scaling
	// centring: avg = (13+3+1)/3=5; v1=-2+5, v2=2+5; u3=-13+5
	require.Equal(t, big.NewInt(3), snapshot.Validators[0].ProposerPriority)
	require.Equal(t, big.NewInt(7), snapshot.Validators[1].ProposerPriority)
	require.Equal(t, big.NewInt(-8), snapshot.Validators[2].ProposerPriority)
}
