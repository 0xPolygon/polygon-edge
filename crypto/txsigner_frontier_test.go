package crypto

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestFrontierSigner(t *testing.T) {
	signer := &FrontierSigner{}

	toAddress := types.StringToAddress("1")
	key, err := GenerateECDSAKey()
	assert.NoError(t, err)

	txn := types.NewTx(&types.LegacyTx{
		GasPrice: big.NewInt(0),
		BaseTx: &types.BaseTx{
			To:    &toAddress,
			Value: big.NewInt(10),
		},
	})
	signedTx, err := signer.SignTx(txn, key)
	assert.NoError(t, err)

	from, err := signer.Sender(signedTx)
	assert.NoError(t, err)
	assert.Equal(t, from, PubKeyToAddress(&key.PublicKey))
}
