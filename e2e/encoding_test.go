package e2e

import (
	"testing"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3/testutil"
)

func TestEncoding(t *testing.T) {
	fr := framework.NewTestServerFromGenesis(t)

	// deploy a contract
	cc := &testutil.Contract{}
	cc.AddEvent(testutil.NewEvent("A").
		Add("address", true).
		Add("address", true))

	cc.EmitEvent("setA1", "A", addr0.String(), addr1.String())
	cc.EmitEvent("setA2", "A", addr1.String(), addr0.String())

	_, addr := fr.DeployContract(cc)

	// send a transaction
	receipt := fr.TxnTo(addr, "setA1")

	// try to get the transaction
	client := fr.JSONRPC().Eth()

	_, err := client.GetTransactionByHash(receipt.TransactionHash)
	assert.NoError(t, err)

	_, err = client.GetBlockByHash(receipt.BlockHash, true)
	assert.NoError(t, err)
}
