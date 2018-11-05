package parity

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/golang/snappy"
)

// SnapshotData is the data of the snapshot
type SnapshotData struct {
	Number     uint64
	Hash       common.Hash
	Difficulty *big.Int
	Pairs      []*pair `rlp:"tail"`
}

type pair struct {
	Block    *abridgedBlock
	Receipts []*types.Receipt
}

type abridgedBlock struct {
	Author       common.Address
	StateRoot    common.Hash
	LogBloom     types.Bloom
	Difficulty   *big.Int
	GasLimit     uint64
	GasUsed      uint64
	Timestamp    *big.Int
	ExtraData    []byte
	Transactions []*types.Transaction
	Uncles       []*types.Header
	MixHash      []byte
	Nonce        []byte
}

// Encode encodes the snapshot data into RLP format
func (s *SnapshotData) Encode() ([]byte, error) {
	data, err := rlp.EncodeToBytes(s)
	if err != nil {
		return nil, err
	}

	data = snappy.Encode(nil, data)

	var obj struct {
		Rest []byte `rlp:"tail"`
	}
	obj.Rest = data

	data, err = rlp.EncodeToBytes(obj)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// DecodeSnapshot decodes RLP byte into a SnapshotData object
func DecodeSnapshot(data []byte) (*SnapshotData, error) {
	var obj struct {
		Rest []byte `rlp:"tail"`
	}
	if err := rlp.DecodeBytes(data, &obj); err != nil {
		return nil, err
	}

	length, err := snappy.DecodedLen(obj.Rest)
	if err != nil {
		return nil, err
	}

	data, err = snappy.Decode(nil, obj.Rest)
	if err != nil {
		return nil, err
	}

	data = data[:length]

	var snapshot SnapshotData
	if err := rlp.DecodeBytes(data, &snapshot); err != nil {
		return nil, err
	}

	// sanity check
	for _, pair := range snapshot.Pairs {
		if len(pair.Block.Transactions) != len(pair.Receipts) {
			return nil, fmt.Errorf("The number of receipts and transaction dont match")
		}

		// check tx hash
		for indx := range pair.Block.Transactions {
			if pair.Block.Transactions[indx].Hash() != pair.Receipts[indx].TxHash {
				return nil, fmt.Errorf("Tx and receipts dont match")
			}
		}
	}

	return &snapshot, nil
}

// ManifestData is the manifest of the snapshots
type ManifestData struct {
	Version     uint64
	StateHashes []common.Hash
	BlockHashes []common.Hash
	StateRoot   common.Hash
	BlockNumber *big.Int
	BlockHash   common.Hash
}
