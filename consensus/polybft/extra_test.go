package polybft

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	mrand "math/rand"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/fastrlp"
)

func TestExtra_Encoding(t *testing.T) {
	t.Parallel()

	digest := crypto.Keccak256([]byte("Dummy content to sign"))
	keys := createRandomTestKeys(t, 2)
	parentSig, err := keys[0].Sign(digest)
	require.NoError(t, err)

	committedSig, err := keys[1].Sign(digest)
	require.NoError(t, err)

	bmp := bitmap.Bitmap{}
	bmp.Set(1)
	bmp.Set(4)

	addedValidators := validator.NewTestValidatorsWithAliases(t, []string{"A", "B", "C"}).GetPublicIdentities()

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
				Validators: &validator.ValidatorSetDelta{},
				Parent:     &Signature{},
				Committed:  &Signature{},
			},
		},
		{
			&Extra{
				Validators: &validator.ValidatorSetDelta{},
			},
		},
		{
			&Extra{
				Validators: &validator.ValidatorSetDelta{
					Added: addedValidators,
				},
				Parent:    &Signature{},
				Committed: &Signature{},
			},
		},
		{
			&Extra{
				Validators: &validator.ValidatorSetDelta{
					Removed: removedValidators,
				},
				Parent:    &Signature{AggregatedSignature: parentSig, Bitmap: bmp},
				Committed: &Signature{},
			},
		},
		{
			&Extra{
				Validators: &validator.ValidatorSetDelta{
					Added:   addedValidators,
					Updated: addedValidators[1:],
					Removed: removedValidators,
				},
				Parent:    &Signature{},
				Committed: &Signature{AggregatedSignature: committedSig, Bitmap: bmp},
			},
		},
		{
			&Extra{
				Parent:    &Signature{AggregatedSignature: parentSig, Bitmap: bmp},
				Committed: &Signature{AggregatedSignature: committedSig, Bitmap: bmp},
			},
		},
		{
			&Extra{
				Parent:    &Signature{AggregatedSignature: parentSig, Bitmap: bmp},
				Committed: &Signature{AggregatedSignature: committedSig, Bitmap: bmp},
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
		require.ErrorContains(t, extra.UnmarshalRLPWith(ar.NewArray()), "incorrect elements count to decode Extra, expected 4 but found 0")
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
		deltaMarshalled := new(validator.ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Parent
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Committed
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})

	t.Run("Incorrect Parent signatures marshalled", func(t *testing.T) {
		t.Parallel()

		extra := &Extra{}
		ar := &fastrlp.Arena{}
		extraMarshalled := ar.NewArray()
		deltaMarshalled := new(validator.ValidatorSetDelta).MarshalRLPWith(ar)
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
		deltaMarshalled := new(validator.ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal

		// Parent
		key, err := wallet.GenerateAccount()
		require.NoError(t, err)

		parentSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash([]byte("This is test hash")), signer.DomainCheckpointManager)
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
		deltaMarshalled := new(validator.ValidatorSetDelta).MarshalRLPWith(ar)
		extraMarshalled.Set(deltaMarshalled)       // ValidatorSetDelta
		extraMarshalled.Set(ar.NewBytes([]byte{})) // Seal

		// Parent
		key, err := wallet.GenerateAccount()
		require.NoError(t, err)

		parentSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash(generateRandomBytes(t)), signer.DomainCheckpointManager)
		extraMarshalled.Set(parentSignature.MarshalRLPWith(ar))

		// Committed
		committedSignature := createSignature(t, []*wallet.Account{key}, types.BytesToHash(generateRandomBytes(t)), signer.DomainCheckpointManager)
		extraMarshalled.Set(committedSignature.MarshalRLPWith(ar))

		// Checkpoint data
		checkpointDataArr := ar.NewArray()
		checkpointDataArr.Set(ar.NewBytes(generateRandomBytes(t)))
		extraMarshalled.Set(checkpointDataArr)

		extra := &Extra{}
		require.Error(t, extra.UnmarshalRLPWith(extraMarshalled))
	})
}

func TestExtra_ValidateFinalizedData_UnhappyPath(t *testing.T) {
	t.Parallel()

	const (
		headerNum = 10
		chainID   = uint64(20)
	)

	header := &types.Header{
		Number: headerNum,
		Hash:   types.BytesToHash(generateRandomBytes(t)),
	}
	parent := &types.Header{
		Number: headerNum - 1,
		Hash:   types.BytesToHash(generateRandomBytes(t)),
	}

	validators := validator.NewTestValidators(t, 6)

	polyBackendMock := new(polybftBackendMock)
	polyBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(nil, errors.New("validators not found"))

	// missing Committed field
	extra := &Extra{}
	err := extra.ValidateFinalizedData(
		header, parent, nil, chainID, nil, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err, fmt.Sprintf("failed to verify signatures for block %d, because signatures are not present", headerNum))

	// missing Checkpoint field
	extra = &Extra{Committed: &Signature{}}
	err = extra.ValidateFinalizedData(
		header, parent, nil, chainID, polyBackendMock, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err, fmt.Sprintf("failed to verify signatures for block %d, because checkpoint data are not present", headerNum))

	// failed to retrieve validators from snapshot
	checkpoint := &CheckpointData{
		EpochNumber: 10,
		BlockRound:  2,
		EventRoot:   types.BytesToHash(generateRandomBytes(t)),
	}
	extra = &Extra{Committed: &Signature{}, Checkpoint: checkpoint}
	err = extra.ValidateFinalizedData(
		header, parent, nil, chainID, polyBackendMock, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err,
		fmt.Sprintf("failed to validate header for block %d. could not retrieve block validators:validators not found", headerNum))

	// failed to verify signatures (quorum not reached)
	polyBackendMock = new(polybftBackendMock)
	polyBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities())

	noQuorumSignature := createSignature(t, validators.GetPrivateIdentities("0", "1"), types.BytesToHash([]byte("FooBar")), signer.DomainCheckpointManager)
	extra = &Extra{Committed: noQuorumSignature, Checkpoint: checkpoint}
	checkpointHash, err := checkpoint.Hash(chainID, headerNum, header.Hash)
	require.NoError(t, err)

	err = extra.ValidateFinalizedData(
		header, parent, nil, chainID, polyBackendMock, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err,
		fmt.Sprintf("failed to verify signatures for block %d (proposal hash %s): quorum not reached", headerNum, checkpointHash))

	// incorrect parent extra size
	validSignature := createSignature(t, validators.GetPrivateIdentities(), checkpointHash, signer.DomainCheckpointManager)
	extra = &Extra{Committed: validSignature, Checkpoint: checkpoint}
	err = extra.ValidateFinalizedData(
		header, parent, nil, chainID, polyBackendMock, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err,
		fmt.Sprintf("failed to verify signatures for block %d: wrong extra size: 0", headerNum))
}

func TestExtra_ValidateParentSignatures(t *testing.T) {
	t.Parallel()

	const (
		chainID   = 15
		headerNum = 23
	)

	polyBackendMock := new(polybftBackendMock)
	polyBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(nil, errors.New("no validators"))

	// validation is skipped for blocks 0 and 1
	extra := &Extra{}
	err := extra.ValidateParentSignatures(
		1, polyBackendMock, nil, nil, nil, chainID, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.NoError(t, err)

	// parent signatures not present
	err = extra.ValidateParentSignatures(
		headerNum, polyBackendMock, nil, nil, nil, chainID, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err, fmt.Sprintf("failed to verify signatures for parent of block %d because signatures are not present", headerNum))

	// validators not found
	validators := validator.NewTestValidators(t, 5)
	incorrectHash := types.BytesToHash([]byte("Hello World"))
	invalidSig := createSignature(t, validators.GetPrivateIdentities(), incorrectHash, signer.DomainCheckpointManager)
	extra = &Extra{Parent: invalidSig}
	err = extra.ValidateParentSignatures(
		headerNum, polyBackendMock, nil, nil, nil, chainID, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err,
		fmt.Sprintf("failed to validate header for block %d. could not retrieve parent validators: no validators", headerNum))

	// incorrect hash is signed
	polyBackendMock = new(polybftBackendMock)
	polyBackendMock.On("GetValidators", mock.Anything, mock.Anything).Return(validators.GetPublicIdentities())

	parent := &types.Header{Number: headerNum - 1, Hash: types.BytesToHash(generateRandomBytes(t))}
	parentCheckpoint := &CheckpointData{EpochNumber: 3, BlockRound: 5}
	parentExtra := &Extra{Checkpoint: parentCheckpoint}

	parentCheckpointHash, err := parentCheckpoint.Hash(chainID, parent.Number, parent.Hash)
	require.NoError(t, err)

	err = extra.ValidateParentSignatures(
		headerNum, polyBackendMock, nil, parent, parentExtra, chainID, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.ErrorContains(t, err,
		fmt.Sprintf("failed to verify signatures for parent of block %d (proposal hash: %s): could not verify aggregated signature", headerNum, parentCheckpointHash))

	// valid signature provided
	validSig := createSignature(t, validators.GetPrivateIdentities(), parentCheckpointHash, signer.DomainCheckpointManager)
	extra = &Extra{Parent: validSig}
	err = extra.ValidateParentSignatures(
		headerNum, polyBackendMock, nil, parent, parentExtra, chainID, signer.DomainCheckpointManager, hclog.NewNullLogger())
	require.NoError(t, err)
}

func TestSignature_Verify(t *testing.T) {
	t.Parallel()

	t.Run("Valid signatures", func(t *testing.T) {
		t.Parallel()

		numValidators := 100
		msgHash := types.Hash{0x1}

		vals := validator.NewTestValidators(t, numValidators)
		validatorsMetadata := vals.GetPublicIdentities()
		validatorSet := vals.ToValidatorSet()

		var signatures bls.Signatures
		bitmap := bitmap.Bitmap{}
		signers := make(map[types.Address]struct{}, len(validatorsMetadata))

		for i, val := range vals.GetValidators() {
			bitmap.Set(uint64(i))

			tempSign, err := val.Account.Bls.Sign(msgHash[:], signer.DomainCheckpointManager)
			require.NoError(t, err)

			signatures = append(signatures, tempSign)
			aggs, err := signatures.Aggregate().Marshal()
			assert.NoError(t, err)

			s := &Signature{
				AggregatedSignature: aggs,
				Bitmap:              bitmap,
			}

			err = s.Verify(10, validatorsMetadata, msgHash, signer.DomainCheckpointManager, hclog.NewNullLogger())
			signers[val.Address()] = struct{}{}

			if !validatorSet.HasQuorum(10, signers) {
				assert.ErrorContains(t, err, "quorum not reached", "failed for %d", i)
			} else {
				assert.NoError(t, err)
			}
		}
	})

	t.Run("Invalid bitmap provided", func(t *testing.T) {
		t.Parallel()

		validatorSet := validator.NewTestValidators(t, 3).GetPublicIdentities()
		bmp := bitmap.Bitmap{}

		// Make bitmap invalid, by setting some flag larger than length of validator set to 1
		bmp.Set(uint64(validatorSet.Len() + 1))
		s := &Signature{Bitmap: bmp}

		err := s.Verify(0, validatorSet, types.Hash{0x1}, signer.DomainCheckpointManager, hclog.NewNullLogger())
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

func TestSignature_VerifyRandom(t *testing.T) {
	t.Parallel()

	numValidators := 100
	vals := validator.NewTestValidators(t, numValidators)
	msgHash := types.Hash{0x1}

	var signature bls.Signatures

	bitmap := bitmap.Bitmap{}
	valIndxsRnd := mrand.Perm(numValidators)[:numValidators*2/3+1]

	accounts := vals.GetValidators()

	for _, index := range valIndxsRnd {
		bitmap.Set(uint64(index))

		tempSign, err := accounts[index].Account.Bls.Sign(msgHash[:], signer.DomainCheckpointManager)
		require.NoError(t, err)

		signature = append(signature, tempSign)
	}

	aggs, err := signature.Aggregate().Marshal()
	require.NoError(t, err)

	s := &Signature{
		AggregatedSignature: aggs,
		Bitmap:              bitmap,
	}

	err = s.Verify(1, vals.GetPublicIdentities(), msgHash, signer.DomainCheckpointManager, hclog.NewNullLogger())
	assert.NoError(t, err)
}

func TestExtra_InitGenesisValidatorsDelta(t *testing.T) {
	t.Parallel()

	t.Run("Happy path", func(t *testing.T) {
		t.Parallel()

		const validatorsCount = 7
		vals := validator.NewTestValidators(t, validatorsCount)

		delta := &validator.ValidatorSetDelta{
			Added:   make(validator.AccountSet, validatorsCount),
			Removed: bitmap.Bitmap{},
		}

		var i int
		for _, val := range vals.Validators {
			delta.Added[i] = &validator.ValidatorMetadata{
				Address:     types.Address(val.Account.Ecdsa.Address()),
				BlsKey:      val.Account.Bls.PublicKey(),
				VotingPower: new(big.Int).SetUint64(val.VotingPower),
			}
			i++
		}

		extra := Extra{Validators: delta}

		genesis := &chain.Genesis{
			ExtraData: extra.MarshalRLPTo(nil),
		}

		genesisExtra, err := GetIbftExtra(genesis.ExtraData)
		assert.NoError(t, err)
		assert.Len(t, genesisExtra.Validators.Added, validatorsCount)
		assert.Empty(t, genesisExtra.Validators.Removed)
	})

	t.Run("Invalid Extra data", func(t *testing.T) {
		t.Parallel()

		genesis := &chain.Genesis{
			ExtraData: append(make([]byte, ExtraVanity), []byte{0x2, 0x3}...),
		}

		_, err := GetIbftExtra(genesis.ExtraData)

		require.Error(t, err)
	})
}

func Test_GetIbftExtraClean(t *testing.T) {
	t.Parallel()

	key, err := wallet.GenerateAccount()
	require.NoError(t, err)

	extra := &Extra{
		Validators: &validator.ValidatorSetDelta{
			Added: validator.AccountSet{
				&validator.ValidatorMetadata{
					Address:     types.BytesToAddress([]byte{11, 22}),
					BlsKey:      key.Bls.PublicKey(),
					VotingPower: new(big.Int).SetUint64(1000),
					IsActive:    true,
				},
			},
		},
		Committed: &Signature{
			AggregatedSignature: []byte{23, 24},
			Bitmap:              []byte{11},
		},
		Parent: &Signature{
			AggregatedSignature: []byte{0, 1},
			Bitmap:              []byte{1},
		},
		Checkpoint: &CheckpointData{
			BlockRound:            1,
			EpochNumber:           1,
			CurrentValidatorsHash: types.BytesToHash([]byte{2, 3}),
			NextValidatorsHash:    types.BytesToHash([]byte{4, 5}),
			EventRoot:             types.BytesToHash([]byte{6, 7}),
		},
	}

	extraClean, err := GetIbftExtraClean(extra.MarshalRLPTo(nil))
	require.NoError(t, err)

	extraTwo := &Extra{}
	require.NoError(t, extraTwo.UnmarshalRLP(extraClean))
	require.True(t, extra.Validators.Equals(extra.Validators))
	require.Equal(t, extra.Checkpoint.BlockRound, extraTwo.Checkpoint.BlockRound)
	require.Equal(t, extra.Checkpoint.EpochNumber, extraTwo.Checkpoint.EpochNumber)
	require.Equal(t, extra.Checkpoint.CurrentValidatorsHash, extraTwo.Checkpoint.CurrentValidatorsHash)
	require.Equal(t, extra.Checkpoint.NextValidatorsHash, extraTwo.Checkpoint.NextValidatorsHash)
	require.Equal(t, extra.Checkpoint.NextValidatorsHash, extraTwo.Checkpoint.NextValidatorsHash)
	require.Equal(t, extra.Parent.AggregatedSignature, extraTwo.Parent.AggregatedSignature)
	require.Equal(t, extra.Parent.Bitmap, extraTwo.Parent.Bitmap)

	require.Nil(t, extraTwo.Committed.AggregatedSignature)
	require.Nil(t, extraTwo.Committed.Bitmap)
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

	currentValidators := validator.NewTestValidators(t, 5).GetPublicIdentities()
	nextValidators := validator.NewTestValidators(t, 3).GetPublicIdentities()

	currentValidatorsHash, err := currentValidators.Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := nextValidators.Hash()
	require.NoError(t, err)

	cases := []struct {
		name                  string
		parentEpochNumber     uint64
		epochNumber           uint64
		currentValidators     validator.AccountSet
		nextValidators        validator.AccountSet
		currentValidatorsHash types.Hash
		nextValidatorsHash    types.Hash
		exitRootHash          types.Hash
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
		{
			name:                  "Invalid exit root hash",
			parentEpochNumber:     2,
			epochNumber:           2,
			currentValidators:     currentValidators,
			nextValidators:        currentValidators,
			currentValidatorsHash: currentValidatorsHash,
			nextValidatorsHash:    currentValidatorsHash,
			exitRootHash:          types.BytesToHash([]byte{0, 1, 2, 3, 4, 5, 6, 7}),
			errString:             "exit root hash not as expected",
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
				EventRoot:             c.exitRootHash,
			}
			parentCheckpoint := &CheckpointData{EpochNumber: c.parentEpochNumber}
			err := checkpoint.Validate(parentCheckpoint, c.currentValidators, c.nextValidators, types.ZeroHash)

			if c.errString != "" {
				require.ErrorContains(t, err, c.errString)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCheckpointData_Copy(t *testing.T) {
	t.Parallel()

	validatorAccs := validator.NewTestValidators(t, 5)
	currentValidatorsHash, err := validatorAccs.GetPublicIdentities("0", "1", "2").Hash()
	require.NoError(t, err)

	nextValidatorsHash, err := validatorAccs.GetPublicIdentities("1", "3", "4").Hash()
	require.NoError(t, err)

	eventRoot := generateRandomBytes(t)
	original := &CheckpointData{
		BlockRound:            1,
		EpochNumber:           5,
		CurrentValidatorsHash: currentValidatorsHash,
		NextValidatorsHash:    nextValidatorsHash,
		EventRoot:             types.BytesToHash(eventRoot),
	}

	copied := original.Copy()
	require.Equal(t, original, copied)
	require.NotSame(t, original, copied)

	// alter arbitrary field on copied instance
	copied.BlockRound = 10
	require.NotEqual(t, original.BlockRound, copied.BlockRound)
}
