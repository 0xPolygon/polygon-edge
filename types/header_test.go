package types

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestHeader_JSON makes sure the Header is properly
// marshalled and unmarshalled from JSON
func TestHeader_JSON(t *testing.T) {
	t.Parallel()

	var (
		//nolint:lll
		headerJSON = `{
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
			}`
		header = Header{
			ParentHash:   Hash{0x1},
			Sha3Uncles:   Hash{0x2},
			Miner:        Address{0x1},
			StateRoot:    Hash{0x4},
			TxRoot:       Hash{0x5},
			ReceiptsRoot: Hash{0x6},
			LogsBloom:    Bloom{0x0},
			Difficulty:   10,
			Number:       11,
			GasLimit:     12,
			GasUsed:      13,
			Timestamp:    14,
			ExtraData:    []byte{97, 98, 99, 100, 101, 102},
			MixHash:      Hash{0x7},
			Nonce:        Nonce{10},
			Hash:         Hash{0x8},
		}
		rg = regexp.MustCompile(`(\t|\n| )+`)
	)

	t.Run("Header marshalled to JSON", func(t *testing.T) {
		t.Parallel()

		marshalledHeader, err := json.Marshal(&header)
		if err != nil {
			t.Fatalf("Unable to marshal header,  %v", err)
		}

		assert.Equal(t, rg.ReplaceAllString(headerJSON, ""), string(marshalledHeader))
	})

	t.Run("Header unmarshalled from JSON", func(t *testing.T) {
		t.Parallel()

		unmarshalledHeader := Header{}
		if err := json.Unmarshal([]byte(headerJSON), &unmarshalledHeader); err != nil {
			t.Fatalf("unable to unmarshall JSON, %v", err)
		}

		assert.Equal(t, header, unmarshalledHeader)
	})
}
