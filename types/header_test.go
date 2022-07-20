package types

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func TestHeaderType_Encode(t *testing.T) {
	header := Header{
		ParentHash:   Hash{0x1},
		Sha3Uncles:   Hash{0x2},
		Miner:        Address{0x1},
		StateRoot:    Hash{0x4},
		TxRoot:       Hash{0x5},
		ReceiptsRoot: Hash{0x6},
		LogsBloom:    Bloom{0x1},
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

	headerJSON, err := json.Marshal(&header)
	if err != nil {
		t.Fatalf("Unable marshall header,  %v", err)
	}

	unmarshallHeader := Header{}
	assert.NoError(t, json.Unmarshal(headerJSON, &unmarshallHeader))

	if !reflect.DeepEqual(unmarshallHeader, header) {
		t.Fatal("Resulting data and expected values are not equal")
	}
}

func TestUnmarshallFromJSON(t *testing.T) {
	testCase := struct {
		name   string
		input  string
		output Header
	}{

		name: "Successfully decode header from json",
		input: `{
				"parentHash": "0x0100000000000000000000000000000000000000000000000000000000000000",
				"sha3Uncles" : "0x0200000000000000000000000000000000000000000000000000000000000000",
				"miner": "0x0100000000000000000000000000000000000000",
				"stateRoot" : "0x0400000000000000000000000000000000000000000000000000000000000000",
				"transactionsRoot" : "0x0500000000000000000000000000000000000000000000000000000000000000",
				"receiptsRoot" : "0x0600000000000000000000000000000000000000000000000000000000000000",
				"logsBloom" : "0x01",
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
		output: Header{
			ParentHash:   Hash{0x1},
			Sha3Uncles:   Hash{0x2},
			Miner:        Address{0x1},
			StateRoot:    Hash{0x4},
			TxRoot:       Hash{0x5},
			ReceiptsRoot: Hash{0x6},
			LogsBloom:    Bloom{0x1},
			Difficulty:   10,
			Number:       11,
			GasLimit:     12,
			GasUsed:      13,
			Timestamp:    14,
			ExtraData:    []byte{97, 98, 99, 100, 101, 102},
			MixHash:      Hash{0x7},
			Nonce:        Nonce{10},
			Hash:         Hash{0x8},
		},
	}

	unmarshallHeader := Header{}

	err := json.Unmarshal([]byte(testCase.input), &unmarshallHeader)
	if err != nil {
		t.Fatalf("Unable to decode header, %v", err)
	}

	if !reflect.DeepEqual(unmarshallHeader, testCase.output) {
		t.Fatal("Resulting data and expected values are not equal")
	}
}
