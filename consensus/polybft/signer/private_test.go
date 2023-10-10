package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivate_Marshal(t *testing.T) {
	t.Parallel()

	blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	require.NoError(t, err)

	// marshal public key
	privateKeyMarshalled, err := blsKey.Marshal()
	require.NoError(t, err)

	// recover private and public key
	blsKeyUnmarshalled, err := UnmarshalPrivateKey(privateKeyMarshalled)
	require.NoError(t, err)

	assert.Equal(t, blsKey, blsKeyUnmarshalled)
}
