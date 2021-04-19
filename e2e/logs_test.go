package e2e

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/testutil"
)

func TestLogs(t *testing.T) {
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, _ := crypto.ParsePrivateKey(hex.MustDecodeHex(privKeyRaw))
	// addr := crypto.PubKeyToAddress(&key.PublicKey)

	//target := types.StringToAddress("0x1010101010101010101010101010101010101010")
	//signer := crypto.NewEIP155Signer(100)

	fr := &framework.TestServer{
		Config: &framework.TestServerConfig{
			PremineAccts: []*framework.SrvAccount{
				{
					Addr: crypto.PubKeyToAddress(&key.PublicKey),
				},
			},
			JsonRPCPort: 8545,
		},
	}

	fmt.Println(fr)

	/*
		client := fr.JSONRPC()

		latest := web3.Latest

		filter := &web3.LogFilter{}
		filter.SetFromUint64(0)
		filter.To = &latest

		fmt.Println(client.Eth().GetLogs(filter))
	*/

	/*
		cc := &testutil.Contract{}
		cc.AddEvent(testutil.NewEvent("A").
			Add("address", true).
			Add("address", true))

		cc.EmitEvent("setA1", "A", addr0.String(), addr1.String())
		cc.EmitEvent("setA2", "A", addr1.String(), addr0.String())

		_, addr := fr.DeployContract(cc)
		receipt := fr.TxnTo(addr, "setA1")

		fmt.Println(receipt)

		filter := &web3.LogFilter{
			BlockHash: &receipt.BlockHash,
		}
		filter.SetFromUint64(0)
		filter.SetToUint64(5)

		dd, _ := filter.MarshalJSON()
		fmt.Println(string(dd))

		logs, err := client.Eth().GetLogs(filter)
		assert.NoError(t, err)
		assert.Len(t, logs, 1)
	*/

}

func TestNewFilter_Logs(t *testing.T) {
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, _ := crypto.ParsePrivateKey(hex.MustDecodeHex(privKeyRaw))
	// addr := crypto.PubKeyToAddress(&key.PublicKey)

	//target := types.StringToAddress("0x1010101010101010101010101010101010101010")
	//signer := crypto.NewEIP155Signer(100)

	fr := &framework.TestServer{
		Config: &framework.TestServerConfig{
			PremineAccts: []*framework.SrvAccount{
				{
					Addr: crypto.PubKeyToAddress(&key.PublicKey),
				},
			},
			JsonRPCPort: 8545,
		},
	}

	cc := &testutil.Contract{}
	cc.AddEvent(testutil.NewEvent("A").
		Add("address", true).
		Add("address", true))

	cc.EmitEvent("setA1", "A", addr0.String(), addr1.String())
	cc.EmitEvent("setA2", "A", addr1.String(), addr0.String())

	_, addr := fr.DeployContract(cc)

	client := fr.JSONRPC()
	id, err := client.Eth().NewFilter(&web3.LogFilter{})
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		fr.TxnTo(addr, "setA1")
	}

	res, err := client.Eth().GetFilterChanges(id)
	assert.NoError(t, err)
	assert.Len(t, res, 10)
}

func TestNewFilter_Block(t *testing.T) {
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, _ := crypto.ParsePrivateKey(hex.MustDecodeHex(privKeyRaw))
	addr := crypto.PubKeyToAddress(&key.PublicKey)

	target := web3.HexToAddress("0x1010101010101010101010101010101010101010")
	//signer := crypto.NewEIP155Signer(100)

	fr := &framework.TestServer{
		Config: &framework.TestServerConfig{
			PremineAccts: []*framework.SrvAccount{
				{
					Addr: crypto.PubKeyToAddress(&key.PublicKey),
				},
			},
			JsonRPCPort: 8545,
		},
	}

	client := fr.JSONRPC()

	id, err := client.Eth().NewBlockFilter()
	assert.NoError(t, err)

	for i := 0; i < 3; i++ {
		root, err := client.Eth().SendTransaction(&web3.Transaction{
			From:     web3.HexToAddress(addr.String()),
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
