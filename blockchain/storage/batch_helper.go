package storage

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Batch interface {
	Delete(key []byte)
	Write() error
	Put(k []byte, data []byte)
}

type BatchHelper struct {
	batch Batch
}

func NewBatchHelper(storage Storage) *BatchHelper {
	batch := storage.NewBatch()

	return &BatchHelper{batch: batch}
}

func (b *BatchHelper) WriteHeader(h *types.Header) {
	b.putRlp(HEADER, h.Hash.Bytes(), h)
}

func (b *BatchHelper) WriteBody(hash types.Hash, body *types.Body) {
	b.putRlp(BODY, hash.Bytes(), body)
}

func (b *BatchHelper) WriteHeadHash(h types.Hash) {
	b.putWithPrefix(HEAD, HASH, h.Bytes())
}

func (b *BatchHelper) WriteTxLookup(hash types.Hash, blockHash types.Hash) {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes()).MarshalTo(nil)

	b.putWithPrefix(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

func (b *BatchHelper) WriteHeadNumber(n uint64) {
	b.putWithPrefix(HEAD, NUMBER, common.EncodeUint64ToBytes(n))
}

func (b *BatchHelper) WriteReceipts(hash types.Hash, receipts []*types.Receipt) {
	rr := types.Receipts(receipts)

	b.putRlp(RECEIPTS, hash.Bytes(), &rr)
}

func (b *BatchHelper) WriteCanonicalHeader(h *types.Header, diff *big.Int) {
	b.WriteHeader(h)
	b.WriteHeadHash(h.Hash)
	b.WriteHeadNumber(h.Number)
	b.WriteCanonicalHash(h.Number, h.Hash)
	b.WriteTotalDifficulty(h.Hash, diff)
}

func (b *BatchHelper) WriteCanonicalHash(n uint64, hash types.Hash) {
	b.putWithPrefix(CANONICAL, common.EncodeUint64ToBytes(n), hash.Bytes())
}

func (b *BatchHelper) WriteTotalDifficulty(hash types.Hash, diff *big.Int) {
	b.putWithPrefix(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

func (b *BatchHelper) putRlp(p, k []byte, raw types.RLPMarshaler) {
	var data []byte

	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	b.putWithPrefix(p, k, data)
}

func (b *BatchHelper) putWithPrefix(p, k, data []byte) {
	fullKey := append(append([]byte{}, p...), k...)

	b.batch.Put(fullKey, data)
}

func (b *BatchHelper) WriteBatch() error {
	return b.batch.Write()
}
