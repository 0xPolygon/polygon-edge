package crypto

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestEIP155Signer_Sender(t *testing.T) {
	t.Parallel()

	toAddress := types.StringToAddress("1")

	testTable := []struct {
		name        string
		chainID     *big.Int
		isHomestead bool
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

			signer := NewEIP155Signer(
				testCase.chainID.Uint64(),
				testCase.isHomestead,
			)

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

func TestEIP155Signer_ChainIDMismatch(t *testing.T) {
	chainIDS := []uint64{1, 10, 100}
	toAddress := types.StringToAddress("1")

	for _, chainIDTop := range chainIDS {
		key, keyGenError := GenerateECDSAKey()
		if keyGenError != nil {
			t.Fatalf("Unable to generate key")
		}

		txn := &types.Transaction{
			To:       &toAddress,
			Value:    big.NewInt(1),
			GasPrice: big.NewInt(0),
		}

		signer := NewEIP155Signer(chainIDTop, true)

		signedTx, signErr := signer.SignTx(txn, key)
		if signErr != nil {
			t.Fatalf("Unable to sign transaction")
		}

		for _, chainIDBottom := range chainIDS {
			signerBottom := NewEIP155Signer(chainIDBottom, true)

			recoveredSender, recoverErr := signerBottom.Sender(signedTx)
			if chainIDTop == chainIDBottom {
				// Addresses should match, no error should be present
				assert.NoError(t, recoverErr)

				assert.Equal(t, recoveredSender.String(), PubKeyToAddress(&key.PublicKey).String())
			} else {
				// There should be an error for mismatched chain IDs
				assert.Error(t, recoverErr)
			}
		}
	}
}
