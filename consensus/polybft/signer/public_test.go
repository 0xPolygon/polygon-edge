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
