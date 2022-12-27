package bls

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PrivateMarshal(t *testing.T) {
	t.Parallel()

	blsKey, err := GenerateBlsKey() // structure which holds private/public key pair
	require.NoError(t, err)

	// marshal public key
	privateKeyMarshalled, err := blsKey.MarshalJSON()
	require.NoError(t, err)
	// recover private and public key
	blsKeyUnmarshalled, err := UnmarshalPrivateKey(privateKeyMarshalled)
	require.NoError(t, err)

	assert.Equal(t, blsKey, blsKeyUnmarshalled)
}

func Test_PrivateMarshal1(t *testing.T) {
	messagePoint := new(G1)
	g1 := new(G1)

	err := messagePoint.HashAndMapTo([]byte("ahsfjhsdjfhsdjf"))
	require.NoError(t, err)

	v := messagePoint.SerializeUncompressed()

	err = g1.DeserializeUncompressed(v)

	require.NoError(t, err)

	require.Equal(t, messagePoint, g1)
}

func Test_SolidityCompatibility(t *testing.T) {
	message, _ := hex.DecodeString("abcd")
	pkb, _ := hex.DecodeString("234a5bd47557d86e76eb95d6e7d41f885f24fe450493bec98babd015728a114e18414e8b403c7e67cdd5b51d41952727d8a28de3734ee4a2114b8c282ce5643f22dd40a86c0efa6719c04ccaa78da913b89efe0c05916eaaa3ff1706367313702a1821de9d934b99c2bbf20bdf25ee9a98d6ef34c539f8880a06637eea2cbfa7")
	sgb, _ := hex.DecodeString("2583e262990c4ed1d68077cf180d4c3f71ee397d4ac1208f9aa0c114f31ee86e2f16020f0981a38d7d40d96b2dd3e0152a5003ff591e5a1526d0251a7ab56fcb")

	pk4, err := BytesToBigInt4(pkb)
	require.NoError(t, err)

	sg2, err := BytesToBigInt2(sgb)
	require.NoError(t, err)

	g1, err := G1FromBigInt(sg2)
	require.NoError(t, err)

	g2, err := G2FromBigInt(pk4)
	require.NoError(t, err)

	pub := &PublicKey{p: g2}
	sig := &Signature{p: g1}

	v := sig.Verify(pub, message)
	require.True(t, v)
}
