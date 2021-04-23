package crypto

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestFrontierSigner(t *testing.T) {
	signer := &FrontierSigner{}

	addr0 := types.Address{0x1}
	key, err := GenerateKey()
	assert.NoError(t, err)

	txn := &types.Transaction{
		To:       &addr0,
		Value:    big.NewInt(10),
		GasPrice: big.NewInt(0),
	}
	txn, err = signer.SignTx(txn, key)
	assert.NoError(t, err)

	from, err := signer.Sender(txn)
	assert.NoError(t, err)
	assert.Equal(t, from, PubKeyToAddress(&key.PublicKey))
}

func TestEIP1155Signer(t *testing.T) {
	signer1 := NewEIP155Signer(1)

	addr0 := types.Address{0x1}
	key, err := GenerateKey()
	assert.NoError(t, err)

	txn := &types.Transaction{
		To:       &addr0,
		Value:    big.NewInt(10),
		GasPrice: big.NewInt(0),
	}
	txn, err = signer1.SignTx(txn, key)
	assert.NoError(t, err)

	from, err := signer1.Sender(txn)
	assert.NoError(t, err)
	assert.Equal(t, from, PubKeyToAddress(&key.PublicKey))

	// try to use a signer with another chain id
	signer2 := NewEIP155Signer(2)
	_, err = signer2.Sender(txn)
	assert.Error(t, err)
}
