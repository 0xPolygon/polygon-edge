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
	key, err := GenerateECDSAPrivateKey()
	assert.NoError(t, err)

	txn := types.NewTx(types.NewLegacyTx(
		types.WithGasPrice(big.NewInt(0)),
		types.WithTo(&toAddress),
		types.WithValue(big.NewInt(10)),
	))

	signedTx, err := signer.SignTx(txn, key)
	assert.NoError(t, err)

	from, err := signer.Sender(signedTx)
	assert.NoError(t, err)
	assert.Equal(t, from, PubKeyToAddress(&key.PublicKey))
}
