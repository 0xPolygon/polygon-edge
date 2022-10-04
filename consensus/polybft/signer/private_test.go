package bls

import (
	"fmt"
	"testing"
)

func Test_PrivateMarshal(t *testing.T) {
	// blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	// require.NoError(t, err)
	//
	// // marshal public key
	// privateKeyMarshalled, err := blsKey.MarshalJSON()
	// require.NoError(t, err)
	// // recover private and public key
	// blsKeyUnmarshalled, err := UnmarshalPrivateKey(privateKeyMarshalled)
	// require.NoError(t, err)
	//
	// assert.Equal(t, blsKey, blsKeyUnmarshalled)

	var b []byte

	fmt.Println(len(b))

}
