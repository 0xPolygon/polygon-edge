package precompiled

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func Test_BlsAggSignsVerification(t *testing.T) {
	const numberOfKeys = 8

	b := &blsAggSignsVerification{}
	message := []byte("test message to sign")

	pubKeys, signatures := generatePubKeysAndSignature(t, numberOfKeys, message)

	// correct message, correct signers and signature
	testCase := generateInput(t, message, pubKeys, signatures)
	out, err := b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolTrue, out)

	// empty message
	testCase = generateInput(t, nil, pubKeys, signatures)
	out, err = b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, out)

	// missing signer
	testCase = generateInput(t, message, pubKeys[1:], signatures)
	out, err = b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, out)

	// missing signature
	testCase = generateInput(t, message, pubKeys, signatures[1:])
	out, err = b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, out)

	// invalid signer
	testCase = generateInput(t, message, append(pubKeys[2:], pubKeys[0]), signatures[1:])
	out, err = b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, out)

	// invalid signature
	testCase = generateInput(t, message, pubKeys[1:], append(signatures[2:], signatures[0]))
	out, err = b.run(testCase, types.ZeroAddress, nil)
	require.NoError(t, err)
	assert.Equal(t, abiBoolFalse, out)
}

func generatePubKeysAndSignature(t *testing.T, numKeys int, messageRaw []byte) ([][]byte, bls.Signatures) {
	t.Helper()

	message := types.BytesToHash(messageRaw)

	validators, err := bls.CreateRandomBlsKeys(numKeys)
	require.NoError(t, err)

	pubKeys := make([][]byte, len(validators))
	signatures := make(bls.Signatures, len(validators))

	for i, validator := range validators {
		sign, err := validator.Sign(message[:], signer.DomainStateReceiver)
		require.NoError(t, err)

		pubKeys[i] = validator.PublicKey().Marshal()
		signatures[i] = sign
	}

	return pubKeys, signatures
}

func generateInput(t *testing.T, messageRaw []byte, publicKeys [][]byte, signatures bls.Signatures) []byte {
	t.Helper()

	message := types.BytesToHash(messageRaw)

	aggSig, err := signatures.Aggregate().Marshal()
	require.NoError(t, err)

	blsVerificationPart, err := BlsVerificationABIType.Encode(
		[2]interface{}{publicKeys, []byte{}})
	require.NoError(t, err)

	encoded, err := abi.Encode([]interface{}{message[:], aggSig, blsVerificationPart},
		abi.MustNewType("tuple(bytes32, bytes, bytes)"))
	require.NoError(t, err)

	return encoded
}
