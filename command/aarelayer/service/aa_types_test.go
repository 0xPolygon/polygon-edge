package service

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/wallet"
)

func Test_Sign_And_RecoverSender(t *testing.T) {
	t.Parallel()

	bytes, err := hex.DecodeString("76de0be417b176b6621aec89fb1f73a926825d19e6fd89044adc32b40e200039")
	require.NoError(t, err)

	chainID := int64(100)
	contractAddress := types.StringToAddress("0x3EeDAA8676458Ca87F92c89aB7cAFbC7087c9C1d")
	from := types.StringToAddress("0xb691f2b8c4e87090c7ebd07b30968773f97dfed2")
	to := types.StringToAddress("0xc0ffee254729296a45a3885639ac7e10f9d54979")

	wallet, err := wallet.NewWalletFromPrivKey(bytes)
	require.NoError(t, err)

	require.Equal(t, from, types.Address(wallet.Address()))

	aatx := &AATransaction{
		Transaction: Transaction{
			Nonce: 0,
			From:  from,
			Payload: []Payload{
				{
					To:       &to,
					Value:    big.NewInt(10),
					GasLimit: big.NewInt(21000),
					Input:    []byte{0xd0, 0x9d, 0xe0, 0x8a}, // framework increment
				},
			},
		},
	}

	require.NoError(t, aatx.Sign(contractAddress, chainID, wallet, nil))

	sender := aatx.RecoverSender(contractAddress, chainID, nil)
	require.Equal(t, from, sender)
}
