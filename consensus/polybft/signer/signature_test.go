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
	signature, err := blsKey.Sign(validTestMsg)
	require.NoError(t, err)

	assert.True(t, signature.Verify(blsKey.PublicKey(), validTestMsg))
	assert.False(t, signature.Verify(blsKey.PublicKey(), invalidTestMsg))
}

func Test_AggregatedSignatureSimple(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	bls1, _ := GenerateBlsKey()
	bls2, _ := GenerateBlsKey()
	bls3, _ := GenerateBlsKey()

	sig1, err := bls1.Sign(validTestMsg)
	require.NoError(t, err)
	sig2, err := bls2.Sign(validTestMsg)
	require.NoError(t, err)
	sig3, err := bls3.Sign(validTestMsg)
	require.NoError(t, err)

	signatures := Signatures{sig1, sig2, sig3}
	publicKeys := PublicKeys{bls1.PublicKey(), bls2.PublicKey(), bls3.PublicKey()}

	verified := signatures.Aggregate().Verify(publicKeys.Aggregate(), validTestMsg)
	assert.True(t, verified)

	notVerified := signatures.Aggregate().Verify(publicKeys.Aggregate(), invalidTestMsg)
	assert.False(t, notVerified)
}

func Test_AggregatedSignature(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKeys, err := CreateRandomBlsKeys(participantsNumber)
	require.NoError(t, err)

	allPubs := make([]*PublicKey, len(blsKeys))

	for i, key := range blsKeys {
		allPubs[i] = key.PublicKey()
	}

	var (
		publicKeys PublicKeys
		signatures Signatures
	)

	for _, key := range blsKeys {
		signature, err := key.Sign(validTestMsg)
		require.NoError(t, err)

		signatures = append(signatures, signature)
		publicKeys = append(publicKeys, key.PublicKey())
	}

	aggSignature := signatures.Aggregate()
	aggPubs := publicKeys.Aggregate()

	assert.True(t, aggSignature.Verify(aggPubs, validTestMsg))
	assert.False(t, aggSignature.Verify(aggPubs, invalidTestMsg))
	assert.True(t, aggSignature.VerifyAggregated([]*PublicKey(publicKeys), validTestMsg))
	assert.False(t, aggSignature.VerifyAggregated([]*PublicKey(publicKeys), invalidTestMsg))
}

func TestSignature_BigInt(t *testing.T) {
	t.Parallel()

	validTestMsg := testGenRandomBytes(t, messageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig1, err := bls1.Sign(validTestMsg)
	assert.NoError(t, err)

	_, err = sig1.ToBigInt()
	require.NoError(t, err)
}

func TestSignature_Unmarshal(t *testing.T) {
	t.Parallel()

	validTestMsg := testGenRandomBytes(t, messageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig, err := bls1.Sign(validTestMsg)
	require.NoError(t, err)

	bytes, err := sig.Marshal()
	require.NoError(t, err)

	sig2, err := UnmarshalSignature(bytes)
	require.NoError(t, err)

	assert.Equal(t, sig, sig2)

	_, err = UnmarshalSignature([]byte{})
	assert.Error(t, err)

	_, err = UnmarshalSignature(nil)
	assert.Error(t, err)
}

func TestSignature_UnmarshalInfinityPoint(t *testing.T) {
	_, err := UnmarshalSignature(make([]byte, 64))
	require.Error(t, err, errInfinityPoint)
}

// testGenRandomBytes generates byte array with random data
func testGenRandomBytes(t *testing.T, size int) (blk []byte) {
	t.Helper()

	blk = make([]byte, size)

	_, err := rand.Reader.Read(blk)
	require.NoError(t, err)

	return
}
