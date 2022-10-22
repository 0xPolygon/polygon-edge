package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SingleSign(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	require.NoError(t, err)

	// Sign valid message
	signature, err := blsKey.Sign(validTestMsg)
	assert.NoError(t, err)

	isOk := signature.Verify(blsKey.PublicKey(), validTestMsg)
	assert.True(t, isOk)

	// Verify if invalid message is signed with correct private key. Only use public key for the verification
	// this should fail => isOk = false
	isOk = signature.Verify(blsKey.PublicKey(), invalidTestMsg)
	assert.False(t, isOk)
}

func Test_AggregatedSign(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	keys, err := CreateRandomBlsKeys(participantsNumber) // create keys for validators
	require.NoError(t, err)

	pubKeys := collectPublicKeys(keys)

	var isOk bool

	signatures := Signatures{}
	aggSignature := new(Signature)

	// test all signatures at once
	for i := 0; i < len(keys); i++ {
		sign, err := keys[i].Sign(validTestMsg)
		require.NoError(t, err)

		signatures = append(signatures, sign)

		// verify correctness of IncludeSignature
		aggSignature = aggSignature.Aggregate(sign)

		isOk = aggSignature.VerifyAggregated(pubKeys[:i+1], validTestMsg)
		assert.True(t, isOk)

		isOk = aggSignature.VerifyAggregated(pubKeys[:i+1], invalidTestMsg)
		assert.False(t, isOk)

		// verify correctness of AggregateSignature
		aggSig := (signatures[:i+1]).Aggregate()

		isOk = aggSig.VerifyAggregated(pubKeys[:i+1], validTestMsg)
		assert.True(t, isOk)

		isOk = aggSig.VerifyAggregated(pubKeys[:i+1], invalidTestMsg)
		assert.False(t, isOk)
	}
}
