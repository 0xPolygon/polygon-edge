package bls

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	messageSize        = 5000
	participantsNumber = 64
)

func Test_VerifySignature(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKey, _ := GenerateBlsKey()
	signature := blsKey.Sign(validTestMsg)

	assert.True(t, signature.Verify(blsKey.PublicKey(), validTestMsg))
	assert.False(t, signature.Verify(blsKey.PublicKey(), invalidTestMsg))
}

func Test_AggregatedSignatureSimple(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	bls1, _ := GenerateBlsKey()
	bls2, _ := GenerateBlsKey()
	bls3, _ := GenerateBlsKey()
	sig1 := bls1.Sign(validTestMsg)
	sig2 := bls2.Sign(validTestMsg)
	sig3 := bls3.Sign(validTestMsg)

	bls1.PublicKey().aggregate(bls2.PublicKey())
	bls1.PublicKey().aggregate(bls3.PublicKey())

	sig1.Aggregate(sig2)
	sig1.Aggregate(sig3)

	verified := sig1.Verify(bls1.PublicKey(), validTestMsg)
	assert.True(t, verified)

	notVerified := sig1.Verify(bls1.PublicKey(), invalidTestMsg)
	assert.False(t, notVerified)
}

func Test_AggregatedSignature(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKeys, err := CreateRandomBlsKeys(participantsNumber)
	require.NoError(t, err)

	allPubs := collectPublicKeys(blsKeys)
	aggPubs := aggregatePublicKeys(allPubs)
	allSignatures := Signatures{}

	var manuallyAggSignature *Signature

	for i, key := range blsKeys {
		signature := key.Sign(validTestMsg)

		allSignatures = append(allSignatures, signature)
		if i == 0 {
			manuallyAggSignature = allSignatures[i]
		} else {
			manuallyAggSignature.Aggregate(allSignatures[i])
		}
	}

	aggSignature := allSignatures.Aggregate()

	var manuallyAggPubs *PublicKey

	for i, pubKey := range allPubs {
		if i == 0 {
			manuallyAggPubs = pubKey
		} else {
			manuallyAggPubs.aggregate(pubKey)
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
	t.Parallel()

	validTestMsg := testGenRandomBytes(t, messageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig1 := bls1.Sign(validTestMsg)

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
