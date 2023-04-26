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

	expected := "0c342ea068099e5be20990590ad86163ce3275106ae30681b6b5822df737d1861e569fe79359c8d01ecfe576f652ec970c5b6709260ecf739b0ea61cc1cc6767"
	bytes, _ := hex.DecodeString("3139343634393730313533353434353137333331343333303931343932303731313035313730303336303738373134363131303435323837383335373237343933383834303135343336383231")

	pk, err := bls.UnmarshalPrivateKey(bytes)
	require.NoError(t, err)

	address := types.BytesToAddress((pk.PublicKey().Marshal())[:types.AddressLength])

	signature, err := MakeKOSKSignature(pk, address, bls.DomainValidatorSet)
	require.NoError(t, err)

	signatureBytes, err := signature.Marshal()
	require.NoError(t, err)

	assert.Equal(t, expected, hex.EncodeToString(signatureBytes))
}
