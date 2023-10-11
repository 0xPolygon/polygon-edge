package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublic_Marshal(t *testing.T) {
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

func TestPublic_MarshalUnmarshalText(t *testing.T) {
	t.Parallel()

	key, err := GenerateBlsKey()
	require.NoError(t, err)

	pubKey := key.PublicKey()
	marshaledPubKey, err := pubKey.MarshalText()
	require.NoError(t, err)

	newPubKey := new(PublicKey)

	require.NoError(t, newPubKey.UnmarshalText(marshaledPubKey))
	require.Equal(t, pubKey, newPubKey)
}

func TestPublicKey_UnmarshalInfinityPoint(t *testing.T) {
	_, err := UnmarshalPublicKey(make([]byte, 128))
	require.Error(t, err, errInfinityPoint)
}
