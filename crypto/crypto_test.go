package crypto

import (
	"math/big"
	"strings"
	"testing"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestKeyEncoding(t *testing.T) {
	for i := 0; i < 10; i++ {
		priv, _ := GenerateKey()

		// marshall private key
		buf, err := MarshallPrivateKey(priv)
		assert.NoError(t, err)

		priv0, err := ParsePrivateKey(buf)
		assert.NoError(t, err)

		assert.Equal(t, priv, priv0)

		// marshall public key
		buf = MarshallPublicKey(&priv.PublicKey)

		pub0, err := ParsePublicKey(buf)
		assert.NoError(t, err)

		assert.Equal(t, priv.PublicKey, *pub0)
	}
}

func TestCreate2(t *testing.T) {
	cases := []struct {
		address  string
		salt     string
		initCode string
		result   string
	}{
		{
			"0x0000000000000000000000000000000000000000",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0x00",
			"0x4D1A2e2bB4F88F0250f26Ffff098B0b30B26BF38",
		},
		{
			"0xdeadbeef00000000000000000000000000000000",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0x00",
			"0xB928f69Bb1D91Cd65274e3c79d8986362984fDA3",
		},
		{
			"0xdeadbeef00000000000000000000000000000000",
			"0x000000000000000000000000feed000000000000000000000000000000000000",
			"0x00",
			"0xD04116cDd17beBE565EB2422F2497E06cC1C9833",
		},
		{
			"0x0000000000000000000000000000000000000000",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0xdeadbeef",
			"0x70f2b2914A2a4b783FaEFb75f459A580616Fcb5e",
		},
		{
			"0x00000000000000000000000000000000deadbeef",
			"0x00000000000000000000000000000000000000000000000000000000cafebabe",
			"0xdeadbeef",
			"0x60f3f640a8508fC6a86d45DF051962668E1e8AC7",
		},
		{
			"0x00000000000000000000000000000000deadbeef",
			"0x00000000000000000000000000000000000000000000000000000000cafebabe",
			"0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"0x1d8bfDC5D46DC4f61D6b6115972536eBE6A8854C",
		},
		{
			"0x0000000000000000000000000000000000000000",
			"0x0000000000000000000000000000000000000000000000000000000000000000",
			"0x",
			"0xE33C0C7F7df4809055C3ebA6c09CFe4BaF1BD9e0",
		},
	}

	for _, c := range cases {
		t.Run("", func(t *testing.T) {
			address := types.StringToAddress(c.address)
			initCode := hex.MustDecodeHex(c.initCode)

			saltRaw := hex.MustDecodeHex(c.salt)
			if len(saltRaw) != 32 {
				t.Fatal("Salt length must be 32 bytes")
			}

			salt := [32]byte{}
			copy(salt[:], saltRaw[:])

			res := CreateAddress2(address, salt, initCode)

			// values in the test cases are in EIP155 format, toLower until
			// the EIP155 is done.
			assert.Equal(t, strings.ToLower(c.result), strings.ToLower(res.String()))
		})
	}
}

func TestValidateSignatureValues(t *testing.T) {
	one := big.NewInt(1).Bytes()
	zero := big.NewInt(0).Bytes()

	secp256k1N, _ := new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)

	limit := secp256k1N.Bytes()
	limitMinus1 := new(big.Int).Sub(secp256k1N, big1).Bytes()

	cases := []struct {
		homestead bool
		v         byte
		r         []byte
		s         []byte
		res       bool
	}{
		// correct v, r, s
		{v: 0, r: one, s: one, res: true},
		{v: 1, r: one, s: one, res: true},
		// incorrect v, correct r, s.
		{v: 2, r: one, s: one, res: false},
		{v: 3, r: one, s: one, res: false},
		// incorrect v, incorrect/correct r, s.
		{v: 2, r: zero, s: zero, res: false},
		{v: 2, r: zero, s: one, res: false},
		{v: 2, r: one, s: zero, res: false},
		{v: 2, r: one, s: one, res: false},
		// correct v, incorrent r, s
		{v: 0, r: zero, s: zero, res: false},
		{v: 0, r: zero, s: one, res: false},
		{v: 0, r: one, s: zero, res: false},
		{v: 1, r: zero, s: zero, res: false},
		{v: 1, r: zero, s: one, res: false},
		{v: 1, r: one, s: zero, res: false},
		// incorrect r, s max limit
		{v: 0, r: limit, s: limit, res: false},
		{v: 0, r: limit, s: limitMinus1, res: false},
		{v: 0, r: limitMinus1, s: limit, res: false},
		// correct v, r, s max limit
		{v: 0, r: limitMinus1, s: limitMinus1, res: true},
	}

	for _, c := range cases {
		found := ValidateSignatureValues(c.v, c.r, c.s)
		assert.Equal(t, found, c.res)
	}
}

func TestVV(t *testing.T) {

	ValidateSignatureValues(0, []byte{0x0, 0x1, 0x2, 0x3}, []byte{0x1, 0x2, 0x3})

}
