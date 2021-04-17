package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestEncoding(t *testing.T) {
	fr := &framework.TestServer{
		Config: &framework.TestServerConfig{
			PremineAccts: []*framework.SrvAccount{
				{
					Addr: types.StringToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0b"),
				},
			},
			JsonRPCPort: 8545,
		},
	}

	client := fr.JSONRPC()
	block0, err := client.Eth().GetBlockByNumber(0, false)
	assert.NoError(t, err)

	block1, err := client.Eth().GetBlockByHash(block0.Hash, false)
	assert.NoError(t, err)

	assert.Equal(t, block0, block1)

	// send a transfer
	receipt, err := fr.SendTxn(&web3.Transaction{
		From:  web3.Address(fr.Config.PremineAccts[0].Addr),
		To:    &web3.Address{0x1},
		Value: big.NewInt(1),
	})
	assert.NoError(t, err)

	fmt.Println("-- receipt --")
	fmt.Println(receipt)
	fmt.Println(receipt.TransactionHash)
}
