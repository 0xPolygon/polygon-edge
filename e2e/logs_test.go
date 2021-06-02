package e2e

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestNewFilter_Logs(t *testing.T) {
	fr := framework.NewTestServerFromGenesis(t)

	contractAddr, err := fr.DeployContract(byteCode)
	if err != nil {
		t.Fatal(err)
	}

	client := fr.JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		fr.TxnTo(contractAddr, "setA1")
	}
	time.Sleep(10 * time.Second)

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, 10)
}

func TestNewFilter_Block(t *testing.T) {
	target := web3.HexToAddress("0x1010101010101010101010101010101010101010")

	fr := framework.NewTestServerFromGenesis(t)

	client := fr.JSONRPC()

	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		root, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(fr.Config.PremineAccts[0].Addr.String()),
			To:       &target,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    uint64(i),
		})
		assert.NoError(t, err)
		fmt.Println(root)

		// avoid having more than one txn on the same block
		time.Sleep(1 * time.Second)
	}

	// wait for the last txn to be sealed
	time.Sleep(2 * time.Second)

	// there should be three changes
	blocks, err := client.Eth().GetFilterChangesBlock(id)
	assert.NoError(t, err)
	fmt.Println(blocks)
}
