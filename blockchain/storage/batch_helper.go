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

func (b *BatchHelper) PutHeader(h *types.Header) {
	b.putRlp(HEADER, h.Hash.Bytes(), h)
}

func (b *BatchHelper) PutBody(hash types.Hash, body *types.Body) {
	b.putRlp(BODY, hash.Bytes(), body)
}

func (b *BatchHelper) PutHeadHash(h types.Hash) {
	b.putWithPrefix(HEAD, HASH, h.Bytes())
}

func (b *BatchHelper) PutTxLookup(hash types.Hash, blockHash types.Hash) {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes()).MarshalTo(nil)

	b.putWithPrefix(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

func (b *BatchHelper) PutHeadNumber(n uint64) {
	b.putWithPrefix(HEAD, NUMBER, common.EncodeUint64ToBytes(n))
}

func (b *BatchHelper) PutReceipts(hash types.Hash, receipts []*types.Receipt) {
	rr := types.Receipts(receipts)

	b.putRlp(RECEIPTS, hash.Bytes(), &rr)
}

func (b *BatchHelper) PutCanonicalHeader(h *types.Header, diff *big.Int) {
	b.PutHeader(h)
	b.PutHeadHash(h.Hash)
	b.PutHeadNumber(h.Number)
	b.PutCanonicalHash(h.Number, h.Hash)
	b.PutTotalDifficulty(h.Hash, diff)
}

func (b *BatchHelper) PutCanonicalHash(n uint64, hash types.Hash) {
	b.putWithPrefix(CANONICAL, common.EncodeUint64ToBytes(n), hash.Bytes())
}

func (b *BatchHelper) PutTotalDifficulty(hash types.Hash, diff *big.Int) {
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
