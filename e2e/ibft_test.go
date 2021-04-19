package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/umbracle/go-web3/jsonrpc"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestIbft_Transfer(t *testing.T) {
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, _ := crypto.ParsePrivateKey(hex.MustDecodeHex(privKeyRaw))

	// 0xdf7fd4830f4cc1440b469615e9996e9fde92608f
	addr := crypto.PubKeyToAddress(&key.PublicKey)

	clt, err := jsonrpc.NewClient("http://127.0.0.1:10002") /* http://127.0.0.1:8545 */
	assert.NoError(t, err)

	eth := clt.Eth()
	fmt.Println(eth.BlockNumber())

	signer := crypto.NewEIP155Signer(100)
	target := types.StringToAddress("0x1010101010101010101010101010101010101010")

	for i := 0; i < 3; i++ {
		txn := &types.Transaction{
			From:     addr,
			To:       &target,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		}
		txn, err = signer.SignTx(txn, key)
		if err != nil {
			panic(err)
		}
		data := txn.MarshalRLP()
		fmt.Println(data)

		hash, err := eth.SendRawTransaction(data)
		assert.NoError(t, err)
		fmt.Println(hash)
	}
}
