package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PublicMarshal(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	key, _ := GenerateBlsKey()
	pub := key.PublicKey()

	pub2, err := UnmarshalPublicKeyFromBigInt(pub.ToBigInt())
	require.NoError(t, err)

	require.Equal(t, pub, pub2)
}

func TestPublic_MarshalUnmarshalJSON(t *testing.T) {
	t.Parallel()

	key, err := GenerateBlsKey()
	require.NoError(t, err)

	pubKey := key.PublicKey()
	marshaledPubKey, err := pubKey.MarshalJSON()
	require.NoError(t, err)

	newPubKey := new(PublicKey)

	err = newPubKey.UnmarshalJSON(marshaledPubKey)
	require.NoError(t, err)
	require.Equal(t, pubKey, newPubKey)
}
