package polybft

import (
	"crypto/rand"
	mrand "math/rand"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestExtra_Encoding(t *testing.T) {
	parentStr := []byte("Here is the parent signature")
	committedStr := []byte("Here is the committed signature")
	bitmapStr := []byte("Here are the bitmap bytes")

	addedValidators := newTestValidatorsWithAliases([]string{"A", "B", "C"}).getPublicIdentities()

	removedValidators := bitmap.Bitmap{}
	removedValidators.Set(2)

	// different extra data for marshall/unmarshall
	var cases = []struct {
		extra *Extra
	}{
		{
			&Extra{},
		},
		{
			&Extra{
				Validators: &ValidatorSetDelta{},
				Parent:     &Signature{},
				Committed:  &Signature{},
			},
		},
		{
			&Extra{
				Validators: &ValidatorSetDelta{},
				Seal:       []byte{3, 4},
			},
		},
		{
			&Extra{
				Validators: &ValidatorSetDelta{
					Added: addedValidators,
				},
				Parent:    &Signature{},
				Committed: &Signature{},
			},
		},
		{
			&Extra{
				Validators: &ValidatorSetDelta{
					Removed: removedValidators,
				},
				Parent:    &Signature{AggregatedSignature: parentStr, Bitmap: bitmapStr},
				Committed: &Signature{},
			},
		},
		{
			&Extra{
				Validators: &ValidatorSetDelta{
					Added:   addedValidators,
					Removed: removedValidators,
				},
				Parent:    &Signature{},
				Committed: &Signature{AggregatedSignature: committedStr, Bitmap: bitmapStr},
			},
		},
		{
			&Extra{
				Parent:    &Signature{AggregatedSignature: parentStr, Bitmap: bitmapStr},
				Committed: &Signature{AggregatedSignature: committedStr, Bitmap: bitmapStr},
			},
		},
	}

	for _, c := range cases {
		data := c.extra.MarshalRLPTo(nil)

		extra := &Extra{}
		assert.NoError(t, extra.UnmarshalRLP(data))
		assert.Equal(t, c.extra, extra)
	}
}

func TestExtra_UnmarshalRLPWith_NegativeCases(t *testing.T) {
	t.Run("Incorrect RLP marshalled data type", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		require.Error(t, extra.UnmarshalRLPWith(ar.NewBool(false)))
	})

	t.Run("Incorrect count of RLP marshalled array elements", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		require.ErrorContains(t, extra.UnmarshalRLPWith(ar.NewArray()), "not enough elements to decode extra, expected 4 but found 0")
	})

	t.Run("Incorrect ValidatorSetDelta marshalled", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewBytes([]byte{0x73}))
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Parent
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Committed
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})

	t.Run("Incorrect Seal marshalled", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := new(ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewNull())          // Seal
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Parent
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Committed
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})

	t.Run("Incorrect Parent signatures marshalled", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := new(ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal
		// Parent
		parentArr := ar.NewArray()
		parentArr.Set(ar.NewBytes([]byte{}))
		extraMarshalled.Set(parentArr)
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Committed
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})

	t.Run("Incorrect Committed signatures marshalled", func(t *testing.T) {
		extra := &Extra{}
		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := new(ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal

		// Parent
		key := wallet.GenerateAccount()
		parentSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash([]byte("This is test hash")))
		extraMarshalled.Set(parentSignature.MarshalRLPWith(ar))

		// Committed
		committedArr := ar.NewArray()
		committedArr.Set(ar.NewBytes([]byte{}))
		extraMarshalled.Set(committedArr)
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})
}

func TestSignature_VerifyCommittedFields(t *testing.T) {
	t.Run("Valid signatures", func(t *testing.T) {
		numValidators := 100
		vals := newTestValidators(numValidators)
		msgHash := types.Hash{0x1}

		ac := vals.getPublicIdentities()

		var signatures bls.Signatures
		bitmap := bitmap.Bitmap{}

		for i, val := range vals.getValidators() {
			bitmap.Set(uint64(i))

			tempSign, err := val.account.Bls.Sign(msgHash[:])
			require.NoError(t, err)

			signatures = append(signatures, tempSign)
			aggs, err := signatures.Aggregate().Marshal()
			assert.NoError(t, err)

			s := &Signature{
				AggregatedSignature: aggs,
				Bitmap:              bitmap,
			}

			err = s.VerifyCommittedFields(ac, msgHash)
			if i+1 < getQuorumSize(numValidators) {
				assert.ErrorContains(t, err, "quorum not reached", "failed for %d", i)
			} else {
				assert.NoError(t, err)
			}
		}
	})

	t.Run("Invalid bitmap provided", func(t *testing.T) {
		validatorSet := newTestValidators(3).getPublicIdentities()
		bmp := bitmap.Bitmap{}

		// Make bitmap invalid, by setting some flag larger than length of validator set to 1
		bmp.Set(uint64(len(validatorSet) + 1))
		s := &Signature{Bitmap: bmp}

		err := s.VerifyCommittedFields(validatorSet, types.Hash{0x1})
		require.Error(t, err)
	})
}

func TestSignature_UnmarshalRLPWith_NegativeCases(t *testing.T) {
	t.Run("Incorrect RLP marshalled data type", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		signature := Signature{}
		require.ErrorContains(t, signature.UnmarshalRLPWith(ar.NewNull()), "array type expected for signature struct")
	})

	t.Run("Incorrect AggregatedSignature field data type", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		signature := Signature{}
		signatureMarshalled := ar.NewArray()
		signatureMarshalled.Set(ar.NewNull())
		signatureMarshalled.Set(ar.NewNull())
		require.ErrorContains(t, signature.UnmarshalRLPWith(signatureMarshalled), "value is not of type bytes")
	})

	t.Run("Incorrect Bitmap field data type", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		signature := Signature{}
		signatureMarshalled := ar.NewArray()
		signatureMarshalled.Set(ar.NewBytes([]byte{0x5, 0x90}))
		signatureMarshalled.Set(ar.NewNull())
		require.ErrorContains(t, signature.UnmarshalRLPWith(signatureMarshalled), "value is not of type bytes")
	})
}

func TestExtra_VerifyCommittedFieldsRandom(t *testing.T) {
	numValidators := 100
	vals := newTestValidators(numValidators)
	msgHash := types.Hash{0x1}

	var signature bls.Signatures

	bitmap := bitmap.Bitmap{}
	valIndxsRnd := mrand.Perm(numValidators)[:numValidators*2/3+1]

	accounts := vals.getValidators()

	for _, index := range valIndxsRnd {
		bitmap.Set(uint64(index))

		tempSign, err := accounts[index].account.Bls.Sign(msgHash[:])
		require.NoError(t, err)

		signature = append(signature, tempSign)
	}

	aggs, err := signature.Aggregate().Marshal()
	require.NoError(t, err)

	s := &Signature{
		AggregatedSignature: aggs,
		Bitmap:              bitmap,
	}

	err = s.VerifyCommittedFields(vals.getPublicIdentities(), msgHash)
	assert.NoError(t, err)
}

func TestExtra_CreateValidatorSetDelta_Cases(t *testing.T) {
	cases := []struct {
		name    string
		oldSet  []string
		newSet  []string
		added   []string
		removed []uint64
	}{
		{
			"Simple",
			[]string{"A", "B", "C", "E", "F"},
			[]string{"B", "E", "H"},
			[]string{"H"},
			[]uint64{0, 2, 4},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vals := newTestValidatorsWithAliases([]string{})

			for _, name := range c.oldSet {
				vals.create(name)
			}
			for _, name := range c.newSet {
				vals.create(name)
			}

			oldValidatorSet := vals.getPublicIdentities(c.oldSet...)
			newValidatorSet := vals.getPublicIdentities(c.newSet...)

			delta, err := createValidatorSetDelta(hclog.NewNullLogger(), oldValidatorSet, newValidatorSet)
			require.NoError(t, err)

			// added items
			assert.Len(t, delta.Added, len(c.added))
			for index, item := range c.added {
				assert.Equal(t, delta.Added[index].Address, vals.getValidator(item).Address())
			}

			// removed items
			for _, i := range c.removed {
				assert.True(t, delta.Removed.IsSet(i))
			}
		})
	}
}

func TestExtra_CreateValidatorSetDelta_BlsDiffer(t *testing.T) {
	vals := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})

	oldValidatorSet := vals.getPublicIdentities("A", "B", "C", "D")

	// change the public bls key of 'B'
	newValidatorSet := vals.getPublicIdentities("B", "E", "F")
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorSet[0].BlsKey = privateKey.PublicKey()

	_, err = createValidatorSetDelta(hclog.NewNullLogger(), oldValidatorSet, newValidatorSet)
	require.Error(t, err)
}

func TestExtra_InitGenesisValidatorsDelta(t *testing.T) {
	t.Run("Happy path", func(t *testing.T) {
		const validatorsCount = 7
		vals := newTestValidators(validatorsCount)

		polyBftConfig := PolyBFTConfig{InitialValidatorSet: vals.getParamValidators()}

		delta := &ValidatorSetDelta{
			Added:   make(AccountSet, validatorsCount),
			Removed: bitmap.Bitmap{},
		}

		var i int
		for _, validator := range vals.validators {
			delta.Added[i] = &ValidatorAccount{
				Address: types.Address(validator.account.Ecdsa.Address()),
				BlsKey:  validator.account.Bls.PublicKey(),
			}
			i++
		}

		extra := Extra{Validators: delta}

		genesis := &chain.Genesis{
			Config: &chain.Params{Engine: map[string]interface{}{
				"polybft": polyBftConfig,
			}},
			ExtraData: append(make([]byte, ExtraVanity), extra.MarshalRLPTo(nil)...),
		}

		genesisExtra, err := GetIbftExtra(genesis.ExtraData)
		assert.NoError(t, err)
		assert.Len(t, genesisExtra.Validators.Added, validatorsCount)
		assert.Empty(t, genesisExtra.Validators.Removed)
	})

	t.Run("Invalid Extra data", func(t *testing.T) {
		validators := newTestValidators(5)
		polyBftConfig := PolyBFTConfig{InitialValidatorSet: validators.getParamValidators()}

		genesis := &chain.Genesis{
			Config: &chain.Params{Engine: map[string]interface{}{
				"polybft": polyBftConfig,
			}},
			ExtraData: append(make([]byte, ExtraVanity), []byte{0x2, 0x3}...),
		}

		_, err := GetIbftExtra(genesis.ExtraData)

		require.Error(t, err)
	})
}

func TestValidatorSetDelta_Copy(t *testing.T) {
	const (
		originalValidatorsCount = 10
		addedValidatorsCount    = 2
	)

	oldValidatorSet := newTestValidators(originalValidatorsCount).getPublicIdentities()
	newValidatorSet := oldValidatorSet[:len(oldValidatorSet)-2]
	originalDelta, err := createValidatorSetDelta(hclog.NewNullLogger(), oldValidatorSet, newValidatorSet)
	require.NoError(t, err)
	require.NotNil(t, originalDelta)
	require.Empty(t, originalDelta.Added)

	copiedDelta := originalDelta.Copy()
	require.NotNil(t, copiedDelta)
	require.NotSame(t, originalDelta, copiedDelta)
	require.NotEqual(t, originalDelta, copiedDelta)
	require.Empty(t, copiedDelta.Added)
	require.Equal(t, copiedDelta.Removed.Len(), originalDelta.Removed.Len())

	newValidators := newTestValidators(addedValidatorsCount).getPublicIdentities()
	copiedDelta.Added = append(copiedDelta.Added, newValidators...)
	require.Empty(t, originalDelta.Added)
	require.Len(t, copiedDelta.Added, addedValidatorsCount)
	require.Equal(t, copiedDelta.Removed.Len(), originalDelta.Removed.Len())
}

func TestValidatorSetDelta_UnmarshalRLPWith_NegativeCases(t *testing.T) {
	t.Run("Incorrect RLP value type provided", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(ar.NewNull()), "value is not of type array")
	})

	t.Run("Empty RLP array provided", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		delta := &ValidatorSetDelta{}
		require.NoError(t, delta.UnmarshalRLPWith(ar.NewArray()))
	})

	t.Run("Incorrect RLP array size", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewBytes([]byte{0x59}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x33}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x26}))
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "incorrect elements count to decode validator set delta, expected 2 but found 3")
	})

	t.Run("Incorrect RLP value type for Added field", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		deltaMarshalled.Set(ar.NewBytes([]byte{0x59}))
		deltaMarshalled.Set(ar.NewBytes([]byte{0x33}))
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "array expected for added validators")
	})

	t.Run("Incorrect RLP value type for ValidatorAccount in Added field", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		addedArray := ar.NewArray()
		addedArray.Set(ar.NewNull())
		deltaMarshalled.Set(addedArray)
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "value is not of type array")
	})

	t.Run("Incorrect RLP value type for Removed field", func(t *testing.T) {
		ar := &fastrlp.Arena{}
		deltaMarshalled := ar.NewArray()
		addedValidators := newTestValidators(3).getPublicIdentities()
		addedArray := ar.NewArray()
		for _, validator := range addedValidators {
			addedArray.Set(validator.MarshalRLPWith(ar))
		}
		deltaMarshalled.Set(addedArray)
		deltaMarshalled.Set(ar.NewNull())
		delta := &ValidatorSetDelta{}
		require.ErrorContains(t, delta.UnmarshalRLPWith(deltaMarshalled), "value is not of type bytes")
	})
}

func Test_GetIbftExtraClean_Fail(t *testing.T) {
	randomBytes := [ExtraVanity]byte{}
	_, err := rand.Read(randomBytes[:])
	require.NoError(t, err)

	extra, err := GetIbftExtraClean(append(randomBytes[:], []byte{0x12, 0x6}...))
	require.Error(t, err)
	require.Nil(t, extra)
}
