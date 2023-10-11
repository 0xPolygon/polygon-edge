package bls

import (
	"encoding/hex"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SingleSign(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	require.NoError(t, err)

	// Sign valid message
	signature, err := blsKey.Sign(validTestMsg, DomainValidatorSet)
	require.NoError(t, err)

	isOk := signature.Verify(blsKey.PublicKey(), validTestMsg, DomainValidatorSet)
	assert.True(t, isOk)

	// Verify if invalid message is signed with correct private key. Only use public key for the verification
	// this should fail => isOk = false
	isOk = signature.Verify(blsKey.PublicKey(), invalidTestMsg, DomainValidatorSet)
	assert.False(t, isOk)
}

func Test_AggregatedSign(t *testing.T) {
	t.Parallel()

	validTestMsg, invalidTestMsg := testGenRandomBytes(t, messageSize), testGenRandomBytes(t, messageSize)

	keys, err := CreateRandomBlsKeys(participantsNumber) // create keys for validators
	require.NoError(t, err)

	pubKeys := make([]*PublicKey, len(keys))

	for i, key := range keys {
		pubKeys[i] = key.PublicKey()
	}

	var isOk bool

	signatures := Signatures{}

	// test all signatures at once
	for i := 0; i < len(keys); i++ {
		sign, err := keys[i].Sign(validTestMsg, DomainValidatorSet)
		require.NoError(t, err)

		signatures = append(signatures, sign)

		// verify correctness of AggregateSignature
		aggSig := signatures.Aggregate()

		isOk = aggSig.VerifyAggregated(pubKeys[:i+1], validTestMsg, DomainValidatorSet)
		assert.True(t, isOk)

		isOk = aggSig.VerifyAggregated(pubKeys[:i+1], invalidTestMsg, DomainValidatorSet)
		assert.False(t, isOk)

		isOk = aggSig.VerifyAggregated(pubKeys[:i+1], validTestMsg, DomainCheckpointManager)
		assert.False(t, isOk)
	}
}

func Test_MakeKOSKSignature(t *testing.T) {
	t.Parallel()

	expected := "03441ebc4bbca37664455b293cfc371ccf9a4f21e40c19a3a25beee77bf6e97d16cdd264c188a01d415ee99865f52567a402044a1c13aa6915a1237a41d35e9e"
	bytes, _ := hex.DecodeString("3139343634393730313533353434353137333331343333303931343932303731313035313730303336303738373134363131303435323837383335373237343933383834303135343336383231")

	pk, err := UnmarshalPrivateKey(bytes)
	require.NoError(t, err)

	supernetManagerAddr := types.StringToAddress("0x1010101")
	address := types.BytesToAddress((pk.PublicKey().Marshal())[:types.AddressLength])

	signature, err := MakeKOSKSignature(pk, address, 10, DomainValidatorSet, supernetManagerAddr)
	require.NoError(t, err)

	signatureBytes, err := signature.Marshal()
	require.NoError(t, err)

	assert.Equal(t, expected, hex.EncodeToString(signatureBytes))

	signature, err = MakeKOSKSignature(pk, address, 100, DomainValidatorSet, supernetManagerAddr)
	require.NoError(t, err)

	signatureBytes, err = signature.Marshal()
	require.NoError(t, err)

	assert.NotEqual(t, expected, hex.EncodeToString(signatureBytes))
}
