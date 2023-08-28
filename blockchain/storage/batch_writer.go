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
	Put(k []byte, v []byte)
}

type BatchWriter struct {
	batch Batch
}

func NewBatchWriter(storage Storage) *BatchWriter {
	return &BatchWriter{batch: storage.NewBatch()}
}

func (b *BatchWriter) PutHeader(h *types.Header) {
	b.putRlp(HEADER, h.Hash.Bytes(), h)
}

func (b *BatchWriter) PutBody(hash types.Hash, body *types.Body) {
	b.putRlp(BODY, hash.Bytes(), body)
}

func (b *BatchWriter) PutHeadHash(h types.Hash) {
	b.putWithPrefix(HEAD, HASH, h.Bytes())
}

func (b *BatchWriter) PutTxLookup(hash types.Hash, blockHash types.Hash) {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes()).MarshalTo(nil)

	b.putWithPrefix(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

func (b *BatchWriter) PutHeadNumber(n uint64) {
	b.putWithPrefix(HEAD, NUMBER, common.EncodeUint64ToBytes(n))
}

func (b *BatchWriter) PutReceipts(hash types.Hash, receipts []*types.Receipt) {
	rr := types.Receipts(receipts)

	b.putRlp(RECEIPTS, hash.Bytes(), &rr)
}

func (b *BatchWriter) PutCanonicalHeader(h *types.Header, diff *big.Int) {
	b.PutHeader(h)
	b.PutHeadHash(h.Hash)
	b.PutHeadNumber(h.Number)
	b.PutCanonicalHash(h.Number, h.Hash)
	b.PutTotalDifficulty(h.Hash, diff)
}

func (b *BatchWriter) PutCanonicalHash(n uint64, hash types.Hash) {
	b.putWithPrefix(CANONICAL, common.EncodeUint64ToBytes(n), hash.Bytes())
}

func (b *BatchWriter) PutTotalDifficulty(hash types.Hash, diff *big.Int) {
	b.putWithPrefix(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

func (b *BatchWriter) PutForks(forks []types.Hash) {
	ff := Forks(forks)

	b.putRlp(FORK, EMPTY, &ff)
}

func (b *BatchWriter) putRlp(p, k []byte, raw types.RLPMarshaler) {
	var data []byte

	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	b.putWithPrefix(p, k, data)
}

func (b *BatchWriter) putWithPrefix(p, k, data []byte) {
	fullKey := append(append(make([]byte, 0, len(p)+len(k)), p...), k...)

	b.batch.Put(fullKey, data)
}

func (b *BatchWriter) WriteBatch() error {
	return b.batch.Write()
}
