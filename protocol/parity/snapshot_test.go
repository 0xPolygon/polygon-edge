package parity

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestSnapshotDataEncoding(t *testing.T) {

	snapshot := &SnapshotData{
		Number:     100,
		Hash:       common.HexToHash("1"),
		Difficulty: big.NewInt(1),
		Pairs: []*pair{
			{
				Block: &abridgedBlock{
					Author:     common.HexToAddress("1"),
					StateRoot:  common.HexToHash("2"),
					Difficulty: big.NewInt(2),
					GasLimit:   222,
					GasUsed:    111,
					Timestamp:  big.NewInt(444),
					Uncles:     []*types.Header{},
					MixHash:    []byte{1, 2, 3},
					Nonce:      []byte{4, 5, 6},
				},
				Receipts: []*types.Receipt{},
			},
		},
	}

	data, err := snapshot.Encode()
	if err != nil {
		t.Fatal(err)
	}

	snapshot2, err := DecodeSnapshot(data)
	if err != nil {
		t.Fatal(err)
	}

	// NOTE. reflect.DeepEqual does not work with the snapshots
	if snapshot2.Number != snapshot.Number {
		t.Fatal("number does not match")
	}
}
