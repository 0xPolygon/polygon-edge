package e2e

import (
	"testing"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
)

func TestEncoding(t *testing.T) {
	fr := framework.NewTestServerFromGenesis(t)

	contractAddr, err := fr.DeployContract(byteCode)
	if err != nil {
		t.Fatal(err)
	}

	// send a transaction
	receipt := fr.TxnTo(contractAddr, "setA1")

	// try to get the transaction
	client := fr.JSONRPC().Eth()

	_, err = client.GetTransactionByHash(receipt.TransactionHash)
	assert.NoError(t, err)

	_, err = client.GetBlockByHash(receipt.BlockHash, true)
	assert.NoError(t, err)
}
