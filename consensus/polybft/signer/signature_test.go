package bls

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	MessageSize        = 5000
	ParticipantsNumber = 64
)

func Test_VerifySignature(t *testing.T) {
	validTestMsg, invalidTestMsg := testGenRandomBytes(t, MessageSize), testGenRandomBytes(t, MessageSize)

	blsKey, _ := GenerateBlsKey()
	signature, err := blsKey.Sign(validTestMsg)
	require.NoError(t, err)

	assert.True(t, signature.Verify(blsKey.PublicKey(), validTestMsg))
	assert.False(t, signature.Verify(blsKey.PublicKey(), invalidTestMsg))
}

func Test_AggregatedSignatureSimple(t *testing.T) {
	validTestMsg, invalidTestMsg := testGenRandomBytes(t, MessageSize), testGenRandomBytes(t, MessageSize)

	bls1, _ := GenerateBlsKey()
	bls2, _ := GenerateBlsKey()
	bls3, _ := GenerateBlsKey()
	sig1, err := bls1.Sign(validTestMsg)
	require.NoError(t, err)
	sig2, err := bls2.Sign(validTestMsg)
	require.NoError(t, err)
	sig3, err := bls3.Sign(validTestMsg)
	require.NoError(t, err)

	verified := sig1.Aggregate(sig2).
		Aggregate(sig3).
		Verify(bls1.PublicKey().aggregate(bls2.PublicKey()).aggregate(bls3.PublicKey()), validTestMsg)
	assert.True(t, verified)

	notVerified := sig1.Aggregate(sig2).
		Aggregate(sig3).
		Verify(bls1.PublicKey().aggregate(bls2.PublicKey()).aggregate(bls3.PublicKey()), invalidTestMsg)
	assert.False(t, notVerified)
}

func Test_AggregatedSignature(t *testing.T) {
	validTestMsg, invalidTestMsg := testGenRandomBytes(t, MessageSize), testGenRandomBytes(t, MessageSize)

	blsKeys, err := CreateRandomBlsKeys(ParticipantsNumber)
	require.NoError(t, err)

	allPubs := collectPublicKeys(blsKeys)
	aggPubs := aggregatePublicKeys(allPubs)
	allSignatures := Signatures{}

	var manuallyAggSignature *Signature

	for i, key := range blsKeys {
		signature, err := key.Sign(validTestMsg)
		require.NoError(t, err)

		allSignatures = append(allSignatures, signature)
		if i == 0 {
			manuallyAggSignature = allSignatures[i]
		} else {
			manuallyAggSignature = manuallyAggSignature.Aggregate(allSignatures[i])
		}
	}

	aggSignature := allSignatures.Aggregate()

	var manuallyAggPubs *PublicKey

	for i, pubKey := range allPubs {
		if i == 0 {
			manuallyAggPubs = pubKey
		} else {
			manuallyAggPubs = manuallyAggPubs.aggregate(pubKey)
		}
	}

	verifed := manuallyAggSignature.Verify(manuallyAggPubs, validTestMsg)
	assert.True(t, verifed)

	verifed = manuallyAggSignature.Verify(manuallyAggPubs, invalidTestMsg)
	assert.False(t, verifed)

	verifed = manuallyAggSignature.Verify(aggPubs, validTestMsg)
	assert.True(t, verifed)

	verifed = manuallyAggSignature.Verify(aggPubs, invalidTestMsg)
	assert.False(t, verifed)

	verifed = aggSignature.Verify(manuallyAggPubs, validTestMsg)
	assert.True(t, verifed)

	verifed = aggSignature.Verify(manuallyAggPubs, invalidTestMsg)
	assert.False(t, verifed)

	verifed = aggSignature.Verify(aggPubs, validTestMsg)
	assert.True(t, verifed)

	verifed = aggSignature.Verify(aggPubs, invalidTestMsg)
	assert.False(t, verifed)
}

func TestSignature_BigInt(t *testing.T) {
	validTestMsg := testGenRandomBytes(t, MessageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig1, err := bls1.Sign(validTestMsg)
	assert.NoError(t, err)

	_, err = sig1.ToBigInt()
	require.NoError(t, err)
}

// testGenRandomBytes generates byte array with random data
func testGenRandomBytes(t *testing.T, size int) (blk []byte) {
	t.Helper()

	blk = make([]byte, size)

	_, err := rand.Reader.Read(blk)
	require.NoError(t, err)

	return
}
