package crypto

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestLondonSignerSender(t *testing.T) {
	t.Parallel()

	toAddress := types.StringToAddress("1")

	testTable := []struct {
		name        string
		chainID     *big.Int
		isGomestead bool
	}{
		{
			"mainnet",
			big.NewInt(1),
			true,
		},
		{
			"expanse mainnet",
			big.NewInt(2),
			true,
		},
		{
			"ropsten",
			big.NewInt(3),
			true,
		},
		{
			"rinkeby",
			big.NewInt(4),
			true,
		},
		{
			"goerli",
			big.NewInt(5),
			true,
		},
		{
			"kovan",
			big.NewInt(42),
			true,
		},
		{
			"geth private",
			big.NewInt(1337),
			true,
		},
		{
			"mega large",
			big.NewInt(0).Exp(big.NewInt(2), big.NewInt(20), nil), // 2**20
			true,
		},
	}

	for _, testCase := range testTable {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			key, keyGenError := GenerateECDSAKey()
			if keyGenError != nil {
				t.Fatalf("Unable to generate key")
			}

			txn := &types.Transaction{
				To:       &toAddress,
				Value:    big.NewInt(1),
				GasPrice: big.NewInt(0),
			}

			chainID := testCase.chainID.Uint64()
			signer := NewLondonSigner(chainID, true, NewEIP155Signer(chainID, true))

			signedTx, signErr := signer.SignTx(txn, key)
			if signErr != nil {
				t.Fatalf("Unable to sign transaction")
			}

			recoveredSender, recoverErr := signer.Sender(signedTx)
			if recoverErr != nil {
				t.Fatalf("Unable to recover sender")
			}

			assert.Equal(t, recoveredSender.String(), PubKeyToAddress(&key.PublicKey).String())
		})
	}
}

func Test_LondonSigner_Sender(t *testing.T) {
	t.Parallel()

	signer := NewLondonSigner(100, true, NewEIP155Signer(100, true))
	to := types.StringToAddress("0xDeaDbeefdEAdbeefdEadbEEFdeadbeEFdEaDbeeF")

	r, ok := big.NewInt(0).SetString("102623819621514684481463796449525884981685455700611671612296611353030973716382", 10)
	require.True(t, ok)

	s, ok := big.NewInt(0).SetString("52694559292202008915948760944211702951173212957828665318138448463580296965840", 10)
	require.True(t, ok)

	testTable := []struct {
		name   string
		tx     *types.Transaction
		sender types.Address
	}{
		{
			name: "sender is 0x85dA99c8a7C2C95964c8EfD687E95E632Fc533D6",
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasPrice:  big.NewInt(1000000402),
				GasTipCap: ethgo.Gwei(1),
				GasFeeCap: ethgo.Gwei(10),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(100000000000000),
				V:         big.NewInt(0),
				R:         r,
				S:         s,
			},
			sender: types.StringToAddress("0x85dA99c8a7C2C95964c8EfD687E95E632Fc533D6"),
		},
	}

	for _, tt := range testTable {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sender, err := signer.Sender(tt.tx)
			require.NoError(t, err)
			require.Equal(t, tt.sender, sender)
		})
	}
}
