package storagev2

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

func (w *Writer) PutHeader(h *types.Header) {
	// block_num_u64 + hash -> header (RLP)
	w.putRlp(HEADER, getKey(h.Number, h.Hash), h)
}

func (w *Writer) PutBody(bn uint64, bh types.Hash, body *types.Body) {
	// block_num_u64 + hash -> body (RLP)
	w.putRlp(BODY, getKey(bn, bh), body)
}

func (w *Writer) PutHeadHash(h types.Hash) {
	w.putIntoTable(HEAD_HASH, HEAD_HASH_KEY, h.Bytes())
}

func (w *Writer) PutHeadNumber(bn uint64) {
	w.putIntoTable(HEAD_NUMBER, HEAD_NUMBER_KEY, common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutTxLookup(hash types.Hash, bn uint64) {
	w.putIntoTable(TX_LOOKUP, hash.Bytes(), common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutBlockLookup(hash types.Hash, bn uint64) {
	w.putIntoTable(BLOCK_LOOKUP, hash.Bytes(), common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutReceipts(bn uint64, bh types.Hash, receipts []*types.Receipt) {
	rs := types.Receipts(receipts)
	w.putRlp(RECEIPTS, getKey(bn, bh), &rs)
}

func (w *Writer) PutCanonicalHeader(h *types.Header, diff *big.Int) {
	w.PutHeader(h)
	w.PutHeadHash(h.Hash)
	w.PutHeadNumber(h.Number)
	w.PutBlockLookup(h.Hash, h.Number)
	w.PutCanonicalHash(h.Number, h.Hash)
	w.PutTotalDifficulty(h.Number, h.Hash, diff)
}

func (w *Writer) PutCanonicalHash(bn uint64, hash types.Hash) {
	w.putIntoTable(CANONICAL, common.EncodeUint64ToBytes(bn), hash.Bytes())
}

func (w *Writer) PutTotalDifficulty(bn uint64, bh types.Hash, diff *big.Int) {
	w.putIntoTable(DIFFICULTY, getKey(bn, bh), diff.Bytes())
}

func (w *Writer) PutForks(forks []types.Hash) {
	fs := Forks(forks)
	w.putRlp(FORK, FORK_KEY, &fs)
}

func (w *Writer) putRlp(t uint8, k []byte, raw types.RLPMarshaler) {
	var data []byte

	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	w.putIntoTable(t, k, data)
}

func (w *Writer) putIntoTable(t uint8, k []byte, data []byte) {
	w.getBatch(t).Put(t, k, data)
}

func (w *Writer) WriteBatch() error {
	for i, b := range w.batch {
		if b != nil {
			err := b.Write()
			if err != nil {
				return err
			}

			w.batch[i] = nil
		}
	}

	return nil
}

func (w *Writer) getBatch(t uint8) Batch {
	i := getIndex(t)
	if w.batch[i] != nil {
		return w.batch[i]
	}

	return w.batch[MAINDB_INDEX]
}

func getKey(n uint64, h types.Hash) []byte {
	return append(append(make([]byte, 0, 40), common.EncodeUint64ToBytes(n)...), h.Bytes()...)
}
