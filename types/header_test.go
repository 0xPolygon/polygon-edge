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
