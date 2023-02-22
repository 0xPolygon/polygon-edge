package helper

import (
	"testing"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MakeKOSKSignature(t *testing.T) {
	t.Parallel()

	expected := "127cfb8e2512b447056f33b91fca6cb2a7039e8b330edc4e5e5287f1c58bba5206373a97c9f09db144c8db5681c39e013ee6039ebbe36e0448e9f704f2d326c0"
	bytes, _ := hex.DecodeString("3139343634393730313533353434353137333331343333303931343932303731313035313730303336303738373134363131303435323837383335373237343933383834303135343336383231")

	pk, err := bls.UnmarshalPrivateKey(bytes)
	require.NoError(t, err)

	address := types.BytesToAddress((pk.PublicKey().Marshal())[:types.AddressLength])

	signature, err := MakeKOSKSignature(pk, address, 10, bls.DomainValidatorSet)
	require.NoError(t, err)

	signatureBytes, err := signature.Marshal()
	require.NoError(t, err)

	assert.Equal(t, expected, hex.EncodeToString(signatureBytes))

	signature, err = MakeKOSKSignature(pk, address, 100, bls.DomainValidatorSet)
	require.NoError(t, err)

	signatureBytes, err = signature.Marshal()
	require.NoError(t, err)

	assert.NotEqual(t, expected, hex.EncodeToString(signatureBytes))
}
