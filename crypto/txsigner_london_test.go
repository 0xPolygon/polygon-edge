package crypto

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/types"
)

func TestLondonSignerSender(t *testing.T) {
	t.Parallel()

	recipient := types.StringToAddress("1")

	tcs := []struct {
		name    string
		chainID *big.Int
		txType  types.TxType
	}{
		{
			"mainnet",
			big.NewInt(1),
			types.LegacyTxType,
		},
		{
			"expanse mainnet",
			big.NewInt(2),
			types.DynamicFeeTxType,
		},
		{
			"ropsten",
			big.NewInt(3),
			types.DynamicFeeTxType,
		},
		{
			"rinkeby",
			big.NewInt(4),
			types.AccessListTxType,
		},
		{
			"goerli",
			big.NewInt(5),
			types.AccessListTxType,
		},
		{
			"kovan",
			big.NewInt(42),
			types.StateTxType,
		},
		{
			"geth private",
			big.NewInt(1337),
			types.StateTxType,
		},
		{
			"mega large",
			big.NewInt(0).Exp(big.NewInt(2), big.NewInt(20), nil), // 2**20
			types.AccessListTxType,
		},
	}

	for _, tc := range tcs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			key, err := GenerateECDSAKey()
			require.NoError(t, err, "unable to generate private key")

			var txn *types.Transaction

			switch tc.txType {
			case types.AccessListTxType:
				txn = types.NewTx(&types.AccessListTxn{
					To:       &recipient,
					Value:    big.NewInt(1),
					GasPrice: big.NewInt(5),
					ChainID:  tc.chainID,
				})
			case types.LegacyTxType:
				txn = types.NewTx(&types.LegacyTx{
					To:       &recipient,
					Value:    big.NewInt(1),
					GasPrice: big.NewInt(5),
				})
			case types.StateTxType:
				txn = types.NewTx(&types.StateTx{
					To:       &recipient,
					Value:    big.NewInt(1),
					GasPrice: big.NewInt(5),
				})
			case types.DynamicFeeTxType:
				txn = types.NewTx(&types.DynamicFeeTx{
					To:      &recipient,
					Value:   big.NewInt(1),
					ChainID: tc.chainID,
				})
			}

			chainID := tc.chainID.Uint64()
			signer := NewLondonSigner(chainID)

			signedTx, err := signer.SignTx(txn, key)
			require.NoError(t, err, "unable to sign transaction")

			sender, err := signer.Sender(signedTx)
			require.NoError(t, err, "failed to recover sender")

			require.Equal(t, sender, PubKeyToAddress(&key.PublicKey))
		})
	}
}

func Test_LondonSigner_Sender(t *testing.T) {
	t.Parallel()

	signer := NewLondonSigner(100)

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
			tx: types.NewTx(&types.DynamicFeeTx{
				GasTipCap: ethgo.Gwei(1),
				GasFeeCap: ethgo.Gwei(10),
				Gas:       21000,
				To:        &to,
				Value:     big.NewInt(100000000000000),
				V:         big.NewInt(0),
				R:         r,
				S:         s,
				ChainID:   big.NewInt(100),
			}),
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
