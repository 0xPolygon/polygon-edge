package bls

import (
	"crypto/rand"
	mRand "math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	bn256 "github.com/umbracle/go-eth-bn256"
)

const (
	messageSize        = 5000
	participantsNumber = 64
)

func Test_VerifySignature(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKey, _ := GenerateBlsKey()
	signature, err := blsKey.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)

	assert.True(t, signature.Verify(blsKey.PublicKey(), validTestMsg, DomainValidatorSet))
	assert.False(t, signature.Verify(blsKey.PublicKey(), invalidTestMsg, DomainValidatorSet))
	assert.False(t, signature.Verify(blsKey.PublicKey(), validTestMsg, DomainCheckpointManager))
}

func Test_VerifySignature_NegativeCases(t *testing.T) {
	t.Parallel()

	// Get a random integer between 1 and 1000
	mRand.Seed(time.Now().UTC().UnixNano())
	messageSize := mRand.Intn(1000) + 1

	validTestMsg := testGenRandomBytes(t, messageSize)

	blsKey, err := GenerateBlsKey()
	require.NoError(t, err)

	signature, err := blsKey.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)

	require.True(t, signature.Verify(blsKey.PublicKey(), validTestMsg, DomainValidatorSet))

	t.Run("Wrong public key", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 100; i++ {
			x, randomG2, err := bn256.RandomG2(rand.Reader)
			require.NoError(t, err)

			publicKey := blsKey.PublicKey()
			publicKey.g2.Add(publicKey.g2, randomG2) // change public key g2 point
			require.False(t, signature.Verify(publicKey, validTestMsg, DomainValidatorSet))

			publicKey = blsKey.PublicKey()
			publicKey.g2.ScalarMult(publicKey.g2, x) // change public key g2 point
			require.False(t, signature.Verify(publicKey, validTestMsg, DomainValidatorSet))
		}
	})

	t.Run("Tampered message", func(t *testing.T) {
		t.Parallel()

		msgCopy := make([]byte, len(validTestMsg))
		copy(msgCopy, validTestMsg)

		for i := 0; i < len(msgCopy); i++ {
			b := msgCopy[i]
			msgCopy[i] = b + 1

			require.False(t, signature.Verify(blsKey.PublicKey(), msgCopy, DomainValidatorSet))
			msgCopy[i] = b
		}
	})

	t.Run("Tampered signature", func(t *testing.T) {
		t.Parallel()

		for i := 0; i < 100; i++ {
			x, randomG1, err := bn256.RandomG1(rand.Reader)
			require.NoError(t, err)

			raw, err := signature.Marshal()
			require.NoError(t, err)

			sigCopy, err := UnmarshalSignature(raw)
			require.NoError(t, err)

			sigCopy.g1.Add(sigCopy.g1, randomG1) // change signature
			require.False(t, sigCopy.Verify(blsKey.PublicKey(), validTestMsg, DomainValidatorSet))

			sigCopy, err = UnmarshalSignature(raw)
			require.NoError(t, err)

			sigCopy.g1.ScalarMult(sigCopy.g1, x) // change signature
			require.False(t, sigCopy.Verify(blsKey.PublicKey(), validTestMsg, DomainValidatorSet))
		}
	})
}

func Test_AggregatedSignatureSimple(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	bls1, _ := GenerateBlsKey()
	bls2, _ := GenerateBlsKey()
	bls3, _ := GenerateBlsKey()

	sig1, err := bls1.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)
	sig2, err := bls2.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)
	sig3, err := bls3.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)

	signatures := Signatures{sig1, sig2, sig3}
	publicKeys := PublicKeys{bls1.PublicKey(), bls2.PublicKey(), bls3.PublicKey()}

	assert.True(t, signatures.Aggregate().Verify(publicKeys.Aggregate(), validTestMsg, DomainValidatorSet))
	assert.False(t, signatures.Aggregate().Verify(publicKeys.Aggregate(), invalidTestMsg, DomainValidatorSet))
	assert.False(t, signatures.Aggregate().Verify(publicKeys.Aggregate(), validTestMsg, DomainCheckpointManager))
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
		signature, err := key.Sign(validTestMsg, DomainValidatorSet)
		require.NoError(t, err)

		signatures = append(signatures, signature)
		publicKeys = append(publicKeys, key.PublicKey())
	}

	aggSignature := signatures.Aggregate()
	aggPubs := publicKeys.Aggregate()

	assert.True(t, aggSignature.Verify(aggPubs, validTestMsg, DomainValidatorSet))
	assert.False(t, aggSignature.Verify(aggPubs, invalidTestMsg, DomainValidatorSet))
	assert.True(t, aggSignature.VerifyAggregated([]*PublicKey(publicKeys), validTestMsg, DomainValidatorSet))
	assert.False(t, aggSignature.VerifyAggregated([]*PublicKey(publicKeys), invalidTestMsg, DomainValidatorSet))
}

func TestSignature_BigInt(t *testing.T) {
	t.Parallel()

	validTestMsg := testGenRandomBytes(t, messageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig1, err := bls1.Sign(validTestMsg, DomainCheckpointManager)
	assert.NoError(t, err)

	_, err = sig1.ToBigInt()
	require.NoError(t, err)
}

func TestSignature_Unmarshal(t *testing.T) {
	t.Parallel()

	validTestMsg := testGenRandomBytes(t, messageSize)

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig, err := bls1.Sign(validTestMsg, DomainCheckpointManager)
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
