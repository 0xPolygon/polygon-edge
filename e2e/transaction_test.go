package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

func TestTransaction(t *testing.T) {

	clt, err := jsonrpc.NewClient("http://127.0.0.1:8545")
	if err != nil {
		t.Fatal(err)
	}

	eth := clt.Eth()
	fmt.Println(eth.BlockNumber())

	root, err := eth.SendTransaction(&web3.Transaction{
		From:     web3.HexToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0b"),
		To:       web3.HexToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0a").String(),
		GasPrice: 10000,
		Gas:      10000000000,
		Value:    big.NewInt(10000),
	})
	assert.NoError(t, err)
	fmt.Println(root)
}
