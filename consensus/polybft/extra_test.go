package polybft

import (
	"crypto/rand"
	"math/big"
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
	t.Parallel()

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
					Updated: addedValidators[1:],
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
		{
			&Extra{
				Parent:    &Signature{AggregatedSignature: parentStr, Bitmap: bitmapStr},
				Committed: &Signature{AggregatedSignature: committedStr, Bitmap: bitmapStr},
				Checkpoint: &CheckpointData{
					BlockRound:            0,
					EpochNumber:           3,
					CurrentValidatorsHash: types.BytesToHash(generateRandomBytes(t)),
					NextValidatorsHash:    types.BytesToHash(generateRandomBytes(t)),
					EventRoot:             types.BytesToHash(generateRandomBytes(t)),
				},
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
	t.Parallel()

	t.Run("Incorrect RLP marshalled data type", func(t *testing.T) {
		t.Parallel()

		extra := &Extra{}
		ar := &fastrlp.Arena{}
		require.Error(t, extra.UnmarshalRLPWith(ar.NewBool(false)))
	})

	t.Run("Incorrect count of RLP marshalled array elements", func(t *testing.T) {
		t.Parallel()

		extra := &Extra{}
		ar := &fastrlp.Arena{}
		require.ErrorContains(t, extra.UnmarshalRLPWith(ar.NewArray()), "incorrect elements count to decode Extra, expected 5 but found 0")
	})

	t.Run("Incorrect ValidatorSetDelta marshalled", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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

	t.Run("Incorrect Checkpoint data marshalled", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := new(ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal

		// Parent
		key := wallet.GenerateAccount()
		parentSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash(generateRandomBytes(t)))
		extraMarshalled.Set(parentSignature.MarshalRLPWith(ar))

		// Committed
		committedSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash(generateRandomBytes(t)))
		extraMarshalled.Set(committedSignature.MarshalRLPWith(ar))

		// Checkpoint data
		checkpointDataArr := ar.NewArray()
		checkpointDataArr.Set(ar.NewBytes(generateRandomBytes(t)))
		extraMarshalled.Set(checkpointDataArr)

		extra := &Extra{}
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})
}

func TestSignature_VerifyCommittedFields(t *testing.T) {
	t.Parallel()

	t.Run("Valid signatures", func(t *testing.T) {
		t.Parallel()

		numValidators := 100
		msgHash := types.Hash{0x1}

		vals := newTestValidators(numValidators)
		validatorsMetadata := vals.getPublicIdentities()
		validatorSet := vals.toValidatorSet()

		var signatures bls.Signatures
		bitmap := bitmap.Bitmap{}
		signers := make(map[types.Address]struct{}, len(validatorsMetadata))

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

			err = s.VerifyCommittedFields(validatorsMetadata, msgHash, hclog.NewNullLogger())
			signers[val.Address()] = struct{}{}

			if !validatorSet.HasQuorum(signers) {
				assert.ErrorContains(t, err, "quorum not reached", "failed for %d", i)
			} else {
				assert.NoError(t, err)
			}
		}
	})

	t.Run("Invalid bitmap provided", func(t *testing.T) {
		t.Parallel()

		validatorSet := newTestValidators(3).getPublicIdentities()
		bmp := bitmap.Bitmap{}

		// Make bitmap invalid, by setting some flag larger than length of validator set to 1
		bmp.Set(uint64(validatorSet.Len() + 1))
		s := &Signature{Bitmap: bmp}

		err := s.VerifyCommittedFields(validatorSet, types.Hash{0x1}, hclog.NewNullLogger())
		require.Error(t, err)
	})
}

func TestSignature_UnmarshalRLPWith_NegativeCases(t *testing.T) {
	t.Parallel()

	t.Run("Incorrect RLP marshalled data type", func(t *testing.T) {
		t.Parallel()

		ar := &fastrlp.Arena{}
		signature := Signature{}
		require.ErrorContains(t, signature.UnmarshalRLPWith(ar.NewNull()), "array type expected for signature struct")
	})

	t.Run("Incorrect AggregatedSignature field data type", func(t *testing.T) {
		t.Parallel()

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
	t.Parallel()

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

	err = s.VerifyCommittedFields(vals.getPublicIdentities(), msgHash, hclog.NewNullLogger())
	assert.NoError(t, err)
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

			vals := newTestValidatorsWithAliases([]string{})

			for _, name := range c.oldSet {
				vals.create(name, 1)
			}
			for _, name := range c.newSet {
				vals.create(name, 1)
			}

			oldValidatorSet := vals.getPublicIdentities(c.oldSet...)
			// update voting power to random value
			maxVotingPower := big.NewInt(100)
			for _, name := range c.updated {
				v := vals.getValidator(name)
				vp, err := rand.Int(rand.Reader, maxVotingPower)
				require.NoError(t, err)
				// make sure generated voting power is different than the original one
				v.votingPower += vp.Uint64() + 1
			}
			newValidatorSet := vals.getPublicIdentities(c.newSet...)

			delta, err := createValidatorSetDelta(oldValidatorSet, newValidatorSet)
			require.NoError(t, err)

			// added validators
			require.Len(t, delta.Added, len(c.added))
			for i, name := range c.added {
				require.Equal(t, delta.Added[i].Address, vals.getValidator(name).Address())
			}

			// removed validators
			for _, i := range c.removed {
				require.True(t, delta.Removed.IsSet(i))
			}

			// updated validators
			require.Len(t, delta.Updated, len(c.updated))
			for i, name := range c.updated {
				require.Equal(t, delta.Updated[i].Address, vals.getValidator(name).Address())
			}
		})
	}
}

func TestExtra_CreateValidatorSetDelta_BlsDiffer(t *testing.T) {
	t.Parallel()

	vals := newTestValidatorsWithAliases([]string{"A", "B", "C", "D", "E", "F"})
	oldValidatorSet := vals.getPublicIdentities("A", "B", "C", "D")

	// change the public bls key of 'B'
	newValidatorSet := vals.getPublicIdentities("B", "E", "F")
	privateKey, err := bls.GenerateBlsKey()
	require.NoError(t, err)

	newValidatorSet[0].BlsKey = privateKey.PublicKey()

	_, err = createValidatorSetDelta(oldValidatorSet, newValidatorSet)
	require.Error(t, err)
}

func TestExtra_InitGenesisValidatorsDelta(t *testing.T) {
	t.Parallel()

	t.Run("Happy path", func(t *testing.T) {
		t.Parallel()

		const validatorsCount = 7
		vals := newTestValidators(validatorsCount)

		polyBftConfig := PolyBFTConfig{InitialValidatorSet: vals.getParamValidators()}

		delta := &ValidatorSetDelta{
			Added:   make(AccountSet, validatorsCount),
			Removed: bitmap.Bitmap{},
		}

		var i int
		for _, validator := range vals.validators {
			delta.Added[i] = &ValidatorMetadata{
				Address:     types.Address(validator.account.Ecdsa.Address()),
				BlsKey:      validator.account.Bls.PublicKey(),
				VotingPower: new(big.Int).SetUint64(validator.votingPower),
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
		t.Parallel()

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
	t.Parallel()

	const (
		originalValidatorsCount = 10
		addedValidatorsCount    = 2
	)

	oldValidatorSet := newTestValidators(originalValidatorsCount).getPublicIdentities()
	newValidatorSet := oldValidatorSet[:len(oldValidatorSet)-2]
	originalDelta, err := createValidatorSetDelta(oldValidatorSet, newValidatorSet)
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
		addedValidators := newTestValidators(3).getPublicIdentities()
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

func Test_GetIbftExtraClean_Fail(t *testing.T) {
	t.Parallel()

	randomBytes := [ExtraVanity]byte{}
	_, err := rand.Read(randomBytes[:])
	require.NoError(t, err)

	extra, err := GetIbftExtraClean(append(randomBytes[:], []byte{0x12, 0x6}...))
	require.Error(t, err)
	require.Nil(t, extra)
}

func TestCheckpointData_Hash(t *testing.T) {
	const (
		chainID     = uint64(1)
		blockNumber = uint64(27)
	)

	blockHash := types.BytesToHash(generateRandomBytes(t))
	origCheckpoint := &CheckpointData{
		BlockRound:            0,
		EpochNumber:           3,
		CurrentValidatorsHash: types.BytesToHash(generateRandomBytes(t)),
		NextValidatorsHash:    types.BytesToHash(generateRandomBytes(t)),
		EventRoot:             types.BytesToHash(generateRandomBytes(t)),
	}
	copyCheckpoint := &CheckpointData{}
	*copyCheckpoint = *origCheckpoint

	origHash, err := origCheckpoint.Hash(chainID, blockNumber, blockHash)
	require.NoError(t, err)

	copyHash, err := copyCheckpoint.Hash(chainID, blockNumber, blockHash)
	require.NoError(t, err)

	require.Equal(t, origHash, copyHash)
}

func TestCheckpointData_Validate(t *testing.T) {
	t.Parallel()

	currentValidators := newTestValidators(5).getPublicIdentities()
	nextValidators := newTestValidators(3).getPublicIdentities()

	currentValidatorsHash, err := currentValidators.Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := nextValidators.Hash()
	require.NoError(t, err)

	cases := []struct {
		name                  string
		parentEpochNumber     uint64
		epochNumber           uint64
		currentValidators     AccountSet
		nextValidators        AccountSet
		currentValidatorsHash types.Hash
		nextValidatorsHash    types.Hash
		errString             string
	}{
		{
			name:                  "Valid (validator set changes)",
			parentEpochNumber:     2,
			epochNumber:           2,
			currentValidators:     currentValidators,
			nextValidators:        nextValidators,
			currentValidatorsHash: currentValidatorsHash,
			nextValidatorsHash:    nextValidatorsHash,
			errString:             "",
		},
		{
			name:                  "Valid (validator set remains the same)",
			parentEpochNumber:     2,
			epochNumber:           2,
			currentValidators:     currentValidators,
			nextValidators:        currentValidators,
			currentValidatorsHash: currentValidatorsHash,
			nextValidatorsHash:    currentValidatorsHash,
			errString:             "",
		},
		{
			name:              "Invalid (gap in epoch numbers)",
			parentEpochNumber: 2,
			epochNumber:       6,
			errString:         "invalid epoch number for epoch-beginning block",
		},
		{
			name:              "Invalid (empty currentValidatorsHash)",
			currentValidators: currentValidators,
			nextValidators:    currentValidators,
			errString:         "current validators hash must not be empty",
		},
		{
			name:                  "Invalid (empty nextValidatorsHash)",
			currentValidators:     currentValidators,
			nextValidators:        currentValidators,
			currentValidatorsHash: currentValidatorsHash,
			errString:             "next validators hash must not be empty",
		},
		{
			name:                  "Invalid (incorrect currentValidatorsHash)",
			currentValidators:     currentValidators,
			nextValidators:        currentValidators,
			currentValidatorsHash: nextValidatorsHash,
			nextValidatorsHash:    nextValidatorsHash,
			errString:             "current validators hashes don't match",
		},
		{
			name:                  "Invalid (incorrect nextValidatorsHash)",
			currentValidators:     nextValidators,
			nextValidators:        nextValidators,
			currentValidatorsHash: nextValidatorsHash,
			nextValidatorsHash:    currentValidatorsHash,
			errString:             "next validators hashes don't match",
		},
		{
			name:                  "Invalid (validator set and epoch numbers change)",
			parentEpochNumber:     2,
			epochNumber:           3,
			currentValidators:     currentValidators,
			nextValidators:        nextValidators,
			currentValidatorsHash: currentValidatorsHash,
			nextValidatorsHash:    nextValidatorsHash,
			errString:             "epoch number should not change for epoch-ending block",
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			checkpoint := &CheckpointData{
				EpochNumber:           c.epochNumber,
				CurrentValidatorsHash: c.currentValidatorsHash,
				NextValidatorsHash:    c.nextValidatorsHash,
			}
			parentCheckpoint := &CheckpointData{EpochNumber: c.parentEpochNumber}
			err := checkpoint.Validate(parentCheckpoint, c.currentValidators, c.nextValidators)

			if c.errString != "" {
				require.ErrorContains(t, err, c.errString)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
