package tracker

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

//nolint
func Test_ValidWSResponse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response []byte
	}{
		{
			"valid response",
			[]byte(`{
				"parentHash": "0x0100000000000000000000000000000000000000000000000000000000000000",
				"sha3Uncles" : "0x0200000000000000000000000000000000000000000000000000000000000000",
				"miner": "0x0100000000000000000000000000000000000000",
				"stateRoot" : "0x0400000000000000000000000000000000000000000000000000000000000000",
				"transactionsRoot" : "0x0500000000000000000000000000000000000000000000000000000000000000",
				"receiptsRoot" : "0x0600000000000000000000000000000000000000000000000000000000000000",
				"logsBloom" : "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"difficulty" : "0xa",
				"number" : "0xb",
				"gasLimit" : "0xc",
				"gasUsed" : "0xd",
				"timestamp" : "0xe",
				"extraData":"0x616263646566",
				"mixHash" : "0x0700000000000000000000000000000000000000000000000000000000000000",
				"nonce" : "0x0a00000000000000",
				"hash" : "0x0800000000000000000000000000000000000000000000000000000000000000"
			}`,
			),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sub := subscription{
				newHeadsCh: make(chan *types.Header, 1),
				errorCh:    make(chan error, 1),
			}

			sub.handleWSResponse(test.response)

			assert.NotNil(t, <-sub.newHeadsCh)
		})
	}
}

//nolint
func Test_InvalidWSResponse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		response []byte
	}{
		{
			"invalid response",
			[]byte(`{
				"ParentHash":"definitely not parent hash",
				"Sha3Uncles":"no uncles",
				"Miner": 123456654321,
				"StateRoot":"0xd4dea16a64d276a4bcf2c558ef35592b00fc2b7e99d4552a9c7c284ea6a28606",
				"ReceiptsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"Difficulty":13685,
				"Number": "number",
				"GasLimit": "2 + 2 = 5",
				"Timestamp":1643881944,
				"ExtraData":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD5AWT4VJSCj",
				"Nonce":"0x0000000000000000",
				"Hash":"0x503dd2ddf02d4c8f7d88d654ed71d1820240fc17f134c5b4c0a2cdc87c410678"
				}`,
			),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			sub := subscription{
				newHeadsCh: make(chan *types.Header, 1),
				errorCh:    make(chan error, 1),
			}

			sub.handleWSResponse(test.response)
			assert.Error(t, <-sub.errorCh)
		})
	}
}
