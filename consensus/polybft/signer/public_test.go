package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PublicMarshal(t *testing.T) {
	blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	require.NoError(t, err)

	pubKey := blsKey.PublicKey() // structure which holds public key

	// marshal public key
	publicKeyMarshalled := pubKey.Marshal()

	// unmarshal public key
	publicKeyUnmarshalled, err := UnmarshalPublicKey(publicKeyMarshalled)
	require.NoError(t, err)

	assert.Equal(t, pubKey, publicKeyUnmarshalled)
	assert.Equal(t, publicKeyMarshalled, publicKeyUnmarshalled.Marshal())
}

func TestPublic_UnmarshalPublicKeyFromBigInt(t *testing.T) {
	key, _ := GenerateBlsKey()
	pub := key.PublicKey()

	res, err := pub.ToBigInt()
	require.NoError(t, err)

	pub2, err := UnmarshalPublicKeyFromBigInt(res)
	require.NoError(t, err)

	require.Equal(t, pub, pub2)
}
