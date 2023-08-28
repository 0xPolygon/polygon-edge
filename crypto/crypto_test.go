package crypto

import (
	"bytes"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyEncoding(t *testing.T) {
	for i := 0; i < 10; i++ {
		priv, _ := GenerateECDSAKey()

		// marshall private key
		buf, err := MarshalECDSAPrivateKey(priv)
		assert.NoError(t, err)

		priv0, err := ParseECDSAPrivateKey(buf)
		assert.NoError(t, err)

		assert.Equal(t, priv, priv0)

		// marshall public key
		buf = MarshalPublicKey(&priv.PublicKey)

		pub0, err := ParsePublicKey(buf)
		assert.NoError(t, err)

		assert.Equal(t, priv.PublicKey, *pub0)
	}
}

func TestCreate2(t *testing.T) {
	t.Parallel()

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
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

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
	t.Parallel()

	var (
		zero     = big.NewInt(0)
		one      = big.NewInt(1)
		two      = big.NewInt(2)
		minusOne = big.NewInt(-1)

		limit       = secp256k1N
		limitMinus1 = new(big.Int).Sub(secp256k1N, one)
		halfLimit   = new(big.Int).Div(secp256k1N, two)
		doubleLimit = new(big.Int).Mul(secp256k1N, two)

		// smaller than secp256k1N but bytes.Compare returns it's bigger
		smallValue, _ = new(big.Int).SetString("fffffffffffffffffffffffffffffffebadd", 16)
	)

	// make sure smallValue is less than secp256k1N by big.Int.Compare
	assert.Equal(
		t,
		limit.Cmp(smallValue),
		1,
		"small value must be less than secp256k1N",
	)

	// make sure smallValue is greater than secp256k1N by bytes.Compare
	assert.Equal(
		t,
		bytes.Compare(smallValue.Bytes(), limit.Bytes()),
		1,
		"small value must be greater than secp256k1N by lexicographical comparison",
	)

	cases := []struct {
		homestead bool
		name      string
		v         *big.Int
		r         *big.Int
		s         *big.Int
		res       bool
	}{
		// correct v, r, s
		{
			name:      "should be valid if v is 0 and r & s are in range",
			homestead: true, v: zero, r: one, s: one, res: true,
		},
		{
			name:      "should be valid if v is 1 and r & s are in range",
			homestead: true, v: one, r: one, s: one, res: true,
		},
		// incorrect v, correct r, s.
		{
			name:      "should be invalid if v is out of range",
			homestead: true, v: two, r: one, s: one, res: false,
		},
		{
			name:      "should be invalid if v is out of range",
			homestead: true, v: big.NewInt(-10), r: one, s: one, res: false,
		},
		{
			name:      "should be invalid if v is out of range",
			homestead: true, v: big.NewInt(10), r: one, s: one, res: false,
		},
		// incorrect v, incorrect/correct r, s.
		{
			name:      "should be invalid if v & r & s are out of range",
			homestead: true, v: two, r: zero, s: zero, res: false,
		},
		{
			name:      "should be invalid if v & r are out of range",
			homestead: true, v: two, r: zero, s: one, res: false,
		},
		{
			name:      "should be invalid if v & s are out of range",
			homestead: true, v: two, r: one, s: zero, res: false,
		},
		// correct v, incorrect r, s
		{
			name:      "should be invalid if r & s are nil",
			homestead: true, v: zero, r: nil, s: nil, res: false,
		},
		{
			name:      "should be invalid if r is nil",
			homestead: true, v: zero, r: nil, s: one, res: false,
		},
		{
			name:      "should be invalid if s is nil",
			homestead: true, v: zero, r: one, s: nil, res: false,
		},
		{
			name:      "should be invalid if r & s are negative",
			homestead: true, v: zero, r: minusOne, s: minusOne, res: false,
		},
		{
			name:      "should be invalid if r is negative",
			homestead: true, v: zero, r: minusOne, s: one, res: false,
		},
		{
			name:      "should be invalid if s is negative",
			homestead: true, v: zero, r: one, s: minusOne, res: false,
		},
		{
			name:      "should be invalid if r & s are out of range",
			homestead: true, v: zero, r: zero, s: zero, res: false,
		},
		{
			name:      "should be invalid if r is out of range (v = 0)",
			homestead: true, v: zero, r: zero, s: one, res: false,
		},
		{
			name:      "should be invalid if s is out of range (v = 0)",
			homestead: true, v: zero, r: one, s: zero, res: false,
		},
		{
			name:      "should be invalid if r & s are out of range (v = 1)",
			homestead: true, v: one, r: zero, s: zero, res: false,
		},
		{
			name:      "should be invalid if r is out of range (v = 1)",
			homestead: true, v: one, r: zero, s: one, res: false,
		},
		{
			name:      "should be invalid if s is out of range (v = 1)",
			homestead: true, v: one, r: one, s: zero, res: false,
		},
		// incorrect r, s max limit (Frontier)
		{
			name:      "should be invalid if r & s equal to secp256k1N in Frontier",
			homestead: false, v: zero, r: limit, s: limit, res: false,
		},
		{
			name:      "should be invalid if r equals to secp256k1N in Frontier",
			homestead: false, v: zero, r: limit, s: limitMinus1, res: false,
		},
		{
			name:      "should be invalid if s equals to secp256k1N in Frontier",
			homestead: false, v: zero, r: limitMinus1, s: limit, res: false,
		},
		// incorrect r, s max limit (Homestead)
		{
			name:      "should be invalid if r & s equal to secp256k1N in Homestead",
			homestead: true, v: zero, r: limit, s: limit, res: false,
		},
		{
			name:      "should be invalid if r equals to secp256k1N in Homestead",
			homestead: true, v: zero, r: limit, s: limitMinus1, res: false,
		},
		{
			name:      "should be invalid if s equals to secp256k1N in Homestead",
			homestead: true, v: zero, r: limitMinus1, s: limit, res: false,
		},
		// frontier v, r, s max limit (Frontier)
		{
			name:      "should be valid if r & s equal to secp256k1N - 1 in Frontier",
			homestead: false, v: zero, r: limitMinus1, s: limitMinus1, res: true,
		},
		// incorrect v, r, s max limit (Homestead)
		{
			name:      "should be invalid if r & s equal to secp256k1N - 1 in Homestead",
			homestead: true, v: zero, r: limitMinus1, s: limitMinus1, res: false,
		},
		// correct v, r, s max limit (Homestead)
		{
			name:      "should be valid if r equals to secp256k1N - 1 and s equals to secp256k1N/2",
			homestead: true, v: zero, r: limitMinus1, s: halfLimit, res: true,
		},
		// edge cases
		// Previously ValidateSignatureValues uses bytes.Compare to compare r and s with upper limit
		// but bytes.Compare compares them lexicographically then it causes strange judgment
		// e.g.
		//   0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141
		// > 0x01fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141 (double value)
		{
			name:      "should be invalid if r & s equal to 2 * secp256k1N in Frontier",
			homestead: false, v: zero, r: doubleLimit, s: doubleLimit, res: false,
		},
		{
			name:      "should be invalid if r equals to 2 * secp256k1N in Frontier",
			homestead: false, v: zero, r: doubleLimit, s: one, res: false,
		},
		{
			name:      "should be invalid if s equals to 2 * secp256k1N in Frontier",
			homestead: false, v: zero, r: one, s: doubleLimit, res: false,
		},
		{
			name:      "should be invalid if r equals to 2 * secp256k1N and s equal to secp256k1N",
			homestead: true, v: zero, r: doubleLimit, s: limit, res: false,
		},
		{
			name:      "should be invalid if r equals to 2 * secp256k1N in Frontier",
			homestead: true, v: zero, r: doubleLimit, s: one, res: false,
		},
		{
			name:      "should be invalid if s equals to secp256k1N in Frontier",
			homestead: true, v: zero, r: one, s: limit, res: false,
		},
		{
			name:      "should be valid if r & s equal to small value",
			homestead: false, v: zero, r: smallValue, s: smallValue, res: true,
		},
		{
			name:      "should be valid if r equals to smallValue in Frontier",
			homestead: false, v: zero, r: smallValue, s: one, res: true,
		},
		{
			name:      "should be valid if s equals to smallValue in Frontier",
			homestead: false, v: zero, r: one, s: smallValue, res: true,
		},
	}

	for _, c := range cases {
		c := c

		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				c.res,
				ValidateSignatureValues(c.v, c.r, c.s, c.homestead),
			)
		})
	}
}

func TestPrivateKeyRead(t *testing.T) {
	t.Parallel()

	// Write private keys to disk, check if read is ok
	testTable := []struct {
		name               string
		privateKeyHex      string
		checksummedAddress string
		shouldFail         bool
	}{
		// Generated with Ganache
		{
			"Valid address #1",
			"0c3d062cd3c642735af6a3c1492d761d39a668a67617a457113eaf50860e9e3f",
			"0x81e83Dc147B81Db5771D998A2C265cc710BE43a5",
			false,
		},
		{
			"Valid address #2",
			"71e6439122f6a44884132d54a978318d7218021a5d8f39fd24f440774d564d87",
			"0xCe1f32314aD63F18123b822a23c214DabAA9F7Cf",
			false,
		},
		{
			"Valid address #3",
			"c6435f6cb3a8f19111737b72944a0b4a7e52d8a6e95f1ebaa2881679f2087709",
			"0x47B7DAc4361062Dfc43d0EA6A2a4C3d27bBcCbdb",
			false,
		},
		{
			"Invalid key",
			"c6435f6cb3a8f19111737b72944a0b4a7e52d8a6e95f1ebaa2881679f",
			"",
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			privateKey, err := BytesToECDSAPrivateKey([]byte(testCase.privateKeyHex))
			if err != nil && !testCase.shouldFail {
				t.Fatalf("Unable to parse private key, %v", err)
			}

			if !testCase.shouldFail {
				address, err := GetAddressFromKey(privateKey)
				if err != nil {
					t.Fatalf("unable to extract key, %v", err)
				}

				assert.Equal(t, testCase.checksummedAddress, address.String())
			} else {
				assert.Nil(t, privateKey)
			}
		})
	}
}

func TestPrivateKeyGeneration(t *testing.T) {
	tempFile := "./privateKeyTesting-" + strconv.FormatInt(time.Now().UTC().Unix(), 10) + ".key"

	t.Cleanup(func() {
		_ = os.Remove(tempFile)
	})

	// Generate the private key and write it to a file
	writtenKey, err := GenerateOrReadPrivateKey(tempFile)
	if err != nil {
		t.Fatalf("Unable to generate private key, %v", err)
	}

	writtenAddress, err := GetAddressFromKey(writtenKey)
	if err != nil {
		t.Fatalf("unable to extract key, %v", err)
	}

	// Read existing key and check if it matches
	readKey, err := GenerateOrReadPrivateKey(tempFile)
	if err != nil {
		t.Fatalf("Unable to read private key, %v", err)
	}

	readAddress, err := GetAddressFromKey(readKey)
	if err != nil {
		t.Fatalf("unable to extract key, %v", err)
	}

	assert.True(t, writtenKey.Equal(readKey))
	assert.Equal(t, writtenAddress.String(), readAddress.String())
}

func TestRecoverPublicKey(t *testing.T) {
	t.Parallel()

	testSignature := []byte{1, 2, 3}

	t.Run("Empty hash", func(t *testing.T) {
		t.Parallel()

		_, err := RecoverPubkey(testSignature, []byte{})
		require.ErrorIs(t, err, errHashOfInvalidLength)
	})

	t.Run("Hash of non appropriate length", func(t *testing.T) {
		t.Parallel()

		_, err := RecoverPubkey(testSignature, []byte{0, 1})
		require.ErrorIs(t, err, errHashOfInvalidLength)
	})

	t.Run("Ok signature and hash", func(t *testing.T) {
		t.Parallel()

		hash := types.BytesToHash([]byte{0, 1, 2})

		privateKey, err := GenerateECDSAKey()
		require.NoError(t, err)

		signature, err := Sign(privateKey, hash.Bytes())
		require.NoError(t, err)

		publicKey, err := RecoverPubkey(signature, hash.Bytes())
		require.NoError(t, err)

		require.True(t, privateKey.PublicKey.Equal(publicKey))
	})
}
