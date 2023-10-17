package signer

import (
	"encoding/hex"
	"testing"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_MakeKOSKSignature(t *testing.T) {
	t.Parallel()

	expected := "03441ebc4bbca37664455b293cfc371ccf9a4f21e40c19a3a25beee77bf6e97d16cdd264c188a01d415ee99865f52567a402044a1c13aa6915a1237a41d35e9e"
	bytes, _ := hex.DecodeString("3139343634393730313533353434353137333331343333303931343932303731313035313730303336303738373134363131303435323837383335373237343933383834303135343336383231")

	pk, err := bls.UnmarshalPrivateKey(bytes)
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
