package validator

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestExtra_CreateValidatorSetDelta_BlsDiffer(t *testing.T) {
	t.Parallel()

	vals := NewTestValidatorsWithAliases(t, []string{"A", "B", "C", "D", "E", "F"})
	oldValidatorSet := vals.GetPublicIdentities("A", "B", "C", "D")

	// change the public bls key of 'B'
	newValidatorSet := vals.GetPublicIdentities("B", "E", "F")
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorSet[0].BlsKey = privateKey.PublicKey()

	_, err = CreateValidatorSetDelta(oldValidatorSet, newValidatorSet)
	require.Error(t, err)
}

func TestValidatorSetDelta_Copy(t *testing.T) {
	t.Parallel()

	const (
		originalValidatorsCount = 10
		addedValidatorsCount    = 2
	)

	oldValidatorSet := NewTestValidators(t, originalValidatorsCount).GetPublicIdentities()
	newValidatorSet := oldValidatorSet[:len(oldValidatorSet)-2]
	originalDelta, err := CreateValidatorSetDelta(oldValidatorSet, newValidatorSet)
	require.NoError(t, err)
	require.NotNil(t, originalDelta)
	require.Empty(t, originalDelta.Added)

	copiedDelta := originalDelta.Copy()
	require.NotNil(t, copiedDelta)
	require.NotSame(t, originalDelta, copiedDelta)
	require.NotEqual(t, originalDelta, copiedDelta)
	require.Empty(t, copiedDelta.Added)
	require.Equal(t, copiedDelta.Removed.Len(), originalDelta.Removed.Len())

	newValidators := NewTestValidators(t, addedValidatorsCount).GetPublicIdentities()
	copiedDelta.Added = append(copiedDelta.Added, newValidators...)
	require.Empty(t, originalDelta.Added)
	require.Len(t, copiedDelta.Added, addedValidatorsCount)
	require.Equal(t, copiedDelta.Removed.Len(), originalDelta.Removed.Len())
}

func TestValidatorSetDelta_UnmarshalRLPWith_NegativeCases(t *testing.T) {
	t.Parallel()

	t.Run("Incorrect RLP value type provided", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(ar.NewNull()), "value is not of type array")
	})

	t.Run("Empty RLP array provided", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		delta := &ValidatorSetDelta{}
		require.NoError(t, delta.UnmarshalRLPWith(ar.NewArray()))
	})

	t.Run("Incorrect RLP array size", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewBytes([]byte{0x59}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x33}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x26}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x74}))
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "incorrect elements count to decode validator set delta, expected 3 but found 4")
	})

	t.Run("Incorrect RLP value type for Added field", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewBytes([]byte{0x59}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x33}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x27}))
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "array expected for added validators")
	})

	t.Run("Incorrect RLP value type for ValidatorMetadata in Added field", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		addedArray := ar.NewArray()
		addedArray.Set(ar.NewNull())
		deltaMarshalled.Set(addedArray)
		deltaMarshalled.Set(ar.NewNullArray())
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "value is not of type array")
	})

	t.Run("Incorrect RLP value type for Removed field", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		addedValidators := NewTestValidators(t, 3).GetPublicIdentities()
		addedArray := ar.NewArray()
		updatedArray := ar.NewArray()
		for _, validator := range addedValidators {
			addedArray.Set(validator.MarshalRLPWith(ar))
		}
		for _, validator := range addedValidators {
			votingPower, err := rand.Int(rand.Reader, big.NewInt(100))
			require.NoError(t, err)

			validator.VotingPower = new(big.Int).Set(votingPower)
			updatedArray.Set(validator.MarshalRLPWith(ar))
		}
		deltaMarshalled.Set(addedArray)
		deltaMarshalled.Set(updatedArray)
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "value is not of type bytes")
	})

	t.Run("Incorrect RLP value type for Updated field", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewArray())
		deltaMarshalled.Set(ar.NewBytes([]byte{0x33}))
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "array expected for updated validators")
	})

	t.Run("Incorrect RLP value type for ValidatorMetadata in Updated field", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		updatedArray := ar.NewArray()
		updatedArray.Set(ar.NewNull())
		deltaMarshalled.Set(ar.NewArray())
		deltaMarshalled.Set(updatedArray)
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "value is not of type array")
	})
}

func TestExtra_CreateValidatorSetDelta_Cases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		oldSet  []string
		newSet  []string
		added   []string
		updated []string
		removed []uint64
	}{
		{
			"Simple",
			[]string{"A", "B", "C", "E", "F"},
			[]string{"B", "E", "H"},
			[]string{"H"},
			[]string{"B", "E"},
			[]uint64{0, 2, 4},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			vals := NewTestValidatorsWithAliases(t, []string{})

			for _, name := range c.oldSet {
				vals.Create(t, name, 1)
			}
			for _, name := range c.newSet {
				vals.Create(t, name, 1)
			}

			oldValidatorSet := vals.GetPublicIdentities(c.oldSet...)
			// update voting power to random value
			maxVotingPower := big.NewInt(100)
			for _, name := range c.updated {
				v := vals.GetValidator(name)
				vp, err := rand.Int(rand.Reader, maxVotingPower)
				require.NoError(t, err)
				// make sure generated voting power is different than the original one
				v.VotingPower += vp.Uint64() + 1
			}
			newValidatorSet := vals.GetPublicIdentities(c.newSet...)

			delta, err := CreateValidatorSetDelta(oldValidatorSet, newValidatorSet)
			require.NoError(t, err)

			// added validators
			require.Len(t, delta.Added, len(c.added))
			for i, name := range c.added {
				require.Equal(t, delta.Added[i].Address, vals.GetValidator(name).Address())
			}

			// removed validators
			for _, i := range c.removed {
				require.True(t, delta.Removed.IsSet(i))
			}

			// updated validators
			require.Len(t, delta.Updated, len(c.updated))
			for i, name := range c.updated {
				require.Equal(t, delta.Updated[i].Address, vals.GetValidator(name).Address())
			}
		})
	}
}
