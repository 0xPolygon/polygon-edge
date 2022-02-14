package tracker

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

//	TODO: example responses are of polygon-edge header type

// nolint
func Test_ValidWSResponse(t *testing.T) {
	testCases := []struct {
		name     string
		response []byte
	}{
		{
			"valid response",
			[]byte(`{
				"ParentHash":"0x38a2600cfed59645953d61cf3e96460c6e2be53e7e4c290baf278d41fe8b3589",
				"Sha3Uncles":"0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
				"Miner":"0x0000000000000000000000000000000000000000",
				"StateRoot":"0xd4dea16a64d276a4bcf2c558ef35592b00fc2b7e99d4552a9c7c284ea6a28606",
				"TxRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"ReceiptsRoot":"0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
				"LogsBloom":"0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
				"Difficulty":13685,
				"Number":13685,
				"GasLimit":5242880,
				"GasUsed":0,
				"Timestamp":1643881944,
				"ExtraData":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD5AWT4VJSCjJvrENymOGDV2tMxXSMNycAwc5Q6iYiS03TRZdr3MYqky4lNFblGkpT5bObOPUpwBKAx+ZirMBHNKU0YdJSprfvKM0BAJHQsrqiKYXei0NSVwbhB1KIePC97JNisZIMPjJbvTXQp/OghMQqZMrdKFESYqHoP84MbMiC256Bm4AXEhqAIbgk2eUwy/WLbYBm8+OUbvwH4ybhB3eVzh1+k6I9OAgdttzkH19ejpiJHkWVdzj2zPNNP0tVs5MkU50r32OHxn7/yFm2ey4/nb9f6b2uPCJqaQGm6BgC4QS7CXQXw4aA4+T0Rth7S723sy6csPnaOujlHoBLBLj23Ous8azoTDrnMHJFRU0B2BYxDetaYaceocGHTYdzZWuMAuEFVJOG5IbLuwOFUX69ZK2SyjOnJWoG1vIi3ztqpHb9IqxBh+Mg3vVuy3/G8yzJ9CY/AeMxfejyH76bkGQfaxTC9AQ==",
				"MixHash":"0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365",
				"Nonce":"0x0000000000000000",
				"Hash":"0x503dd2ddf02d4c8f7d88d654ed71d1820240fc17f134c5b4c0a2cdc87c410678"
				}`,
			),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			sub := subscription{}

			_, err := sub.handleWSResponse(test.response)
			assert.NoError(t, err)
		})
	}
}

// nolint
func Test_InvalidWSResponse(t *testing.T) {
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
				"ExtraData":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD5AWT4VJSCjJvrENymOGDV2tMxXSMNycAwc5Q6iYiS03TRZdr3MYqky4lNFblGkpT5bObOPUpwBKAx+ZirMBHNKU0YdJSprfvKM0BAJHQsrqiKYXei0NSVwbhB1KIePC97JNisZIMPjJbvTXQp/OghMQqZMrdKFESYqHoP84MbMiC256Bm4AXEhqAIbgk2eUwy/WLbYBm8+OUbvwH4ybhB3eVzh1+k6I9OAgdttzkH19ejpiJHkWVdzj2zPNNP0tVs5MkU50r32OHxn7/yFm2ey4/nb9f6b2uPCJqaQGm6BgC4QS7CXQXw4aA4+T0Rth7S723sy6csPnaOujlHoBLBLj23Ous8azoTDrnMHJFRU0B2BYxDetaYaceocGHTYdzZWuMAuEFVJOG5IbLuwOFUX69ZK2SyjOnJWoG1vIi3ztqpHb9IqxBh+Mg3vVuy3/G8yzJ9CY/AeMxfejyH76bkGQfaxTC9AQ==",
				"Nonce":"0x0000000000000000",
				"Hash":"0x503dd2ddf02d4c8f7d88d654ed71d1820240fc17f134c5b4c0a2cdc87c410678"
				}`,
			),
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			sub := subscription{}

			_, err := sub.handleWSResponse(test.response)
			assert.Error(t, err)
		})
	}
}
