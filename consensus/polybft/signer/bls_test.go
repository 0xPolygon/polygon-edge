package bls

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_SolidityCompatibility(t *testing.T) {
	message, _ := hex.DecodeString("abcd")
	pkb, _ := hex.DecodeString("234a5bd47557d86e76eb95d6e7d41f885f24fe450493bec98babd015728a114e18414e8b403c7e67cdd5b51d41952727d8a28de3734ee4a2114b8c282ce5643f22dd40a86c0efa6719c04ccaa78da913b89efe0c05916eaaa3ff1706367313702a1821de9d934b99c2bbf20bdf25ee9a98d6ef34c539f8880a06637eea2cbfa7")
	sgb, _ := hex.DecodeString("2583e262990c4ed1d68077cf180d4c3f71ee397d4ac1208f9aa0c114f31ee86e2f16020f0981a38d7d40d96b2dd3e0152a5003ff591e5a1526d0251a7ab56fcb")

	pk4 := [4]*big.Int{
		new(big.Int).SetBytes(pkb[32:64]),
		new(big.Int).SetBytes(pkb[:32]),
		new(big.Int).SetBytes(pkb[96:]),
		new(big.Int).SetBytes(pkb[64:96]),
	}

	sg2 := [2]*big.Int{
		new(big.Int).SetBytes(sgb[:32]),
		new(big.Int).SetBytes(sgb[32:]),
	}

	pub, err := UnmarshalPublicKeyFromBigInt(pk4)
	require.NoError(t, err)

	sig, err := UnmarshalSignatureFromBigInt(sg2)
	require.NoError(t, err)

	require.True(t, sig.Verify(pub, message))
}

func TestPublic_UnmarshalPublicKeyFromBigInt(t *testing.T) {
	t.Parallel()

	key, _ := GenerateBlsKey()
	pub := key.PublicKey()

	pub2, err := UnmarshalPublicKeyFromBigInt(pub.ToBigInt())
	require.NoError(t, err)

	assert.Equal(t, pub, pub2)
	assert.Equal(t, pub.ToBigInt(), pub2.ToBigInt())
}

func TestSignature_BigInt(t *testing.T) {
	t.Parallel()

	validTestMsg := []byte("this is a message to test")

	bls1, err := GenerateBlsKey()
	require.NoError(t, err)

	sig1, err := bls1.Sign(validTestMsg)
	assert.NoError(t, err)

	_, err = sig1.ToBigInt()
	require.NoError(t, err)
}

func Test_BytesToBigInt2WrongSize(t *testing.T) {
	_, err := BytesToBigInt2(nil)
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 63))
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 65))
	require.Error(t, err)

	_, err = BytesToBigInt2(make([]byte, 64))
	require.NoError(t, err)

	_, err = BytesToBigInt2(append(make([]byte, 63), 12))
	require.NoError(t, err)
}

func Test_BytesToBigInt4WrongSize(t *testing.T) {
	_, err := BytesToBigInt4(nil)
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 127))
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 129))
	require.Error(t, err)

	_, err = BytesToBigInt4(make([]byte, 128))
	require.NoError(t, err)

	_, err = BytesToBigInt4(append(make([]byte, 127), 8))
	require.NoError(t, err)
}
