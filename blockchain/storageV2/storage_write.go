package storageV2

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

func (w *Writer) PutHeader(h *types.Header) {
	w.putRlp(common.EncodeUint64ToBytes(h.Number), HEADER, h)
}

func (w *Writer) PutBody(bn uint64, body *types.Body) {
	w.putRlp(common.EncodeUint64ToBytes(bn), BODY, body)
}

func (w *Writer) PutHeadHash(h types.Hash) {
	w.putWithSuffix(HASH, GIDLID, h.Bytes())
}

func (w *Writer) PutHeadNumber(bn uint64) {
	w.putWithSuffix(NUMBER, GIDLID, common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutTxLookup(hash types.Hash, bn uint64) {
	w.putWithSuffix(hash.Bytes(), GIDLID, common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutBlockLookup(hash types.Hash, bn uint64) {
	w.putWithSuffix(hash.Bytes(), GIDLID, common.EncodeUint64ToBytes(bn))
}

func (w *Writer) PutReceipts(bn uint64, receipts []*types.Receipt) {
	rs := types.Receipts(receipts)
	w.putRlp(common.EncodeUint64ToBytes(bn), RECEIPTS, &rs)
}

func (w *Writer) PutCanonicalHeader(h *types.Header, diff *big.Int) {
	w.PutHeader(h)
	w.PutHeadHash(h.Hash)
	w.PutHeadNumber(h.Number)
	w.PutCanonicalHash(h.Number, h.Hash)
	w.PutTotalDifficulty(h.Number, diff)
}

func (w *Writer) PutCanonicalHash(bn uint64, hash types.Hash) {
	w.putWithSuffix(common.EncodeUint64ToBytes(bn), CANONICAL, hash.Bytes())
}

func (w *Writer) PutTotalDifficulty(bn uint64, diff *big.Int) {
	w.putWithSuffix(common.EncodeUint64ToBytes(bn), DIFFICULTY, diff.Bytes())
}

func (w *Writer) PutForks(forks []types.Hash) {
	fs := Forks(forks)
	w.putRlp(FORK, GIDLID, &fs)
}

func (w *Writer) putRlp(k, mc []byte, raw types.RLPMarshaler) {
	var data []byte

	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	w.putWithSuffix(k, mc, data)
}

func (w *Writer) putWithSuffix(k, mc, data []byte) {
	fullKey := append(append(make([]byte, 0, len(k)+len(mc)), k...), mc...)

	w.getBatch(mc).Put(fullKey, data)
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

func (w *Writer) getBatch(mc []byte) Batch {
	i := getIndex(mc)
	if w.batch[i] != nil {
		return w.batch[i]
	}

	return w.batch[MAINDB_INDEX]
}
