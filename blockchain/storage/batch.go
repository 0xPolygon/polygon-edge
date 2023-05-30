package storage

import (
	"encoding/binary"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/fastrlp"
)

type Batch struct {
	DB   *leveldb.DB
	B    *leveldb.Batch
	Size int
}

func NewBatch(db *leveldb.DB) *Batch {
	return &Batch{
		DB: db,
		B:  new(leveldb.Batch),
	}
}

func (b *Batch) WriteHeader(h *types.Header) error {
	return b.writeRLP(HEADER, h.Hash.Bytes(), h)
}

func (b *Batch) WriteBody(hash types.Hash, body *types.Body) error {
	return b.writeRLP(BODY, hash.Bytes(), body)
}

// Delete inserts the a key removal into the batch for later committing.
func (b *Batch) Delete(key []byte) error {
	b.B.Delete(key)
	b.Size += len(key)

	return nil
}

func (b *Batch) WriteHeadHash(h types.Hash) error {
	return b.add(HEAD, HASH, h.Bytes())
}

func (b *Batch) WriteTxLookup(hash types.Hash, blockHash types.Hash) error {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes())

	return b.write2(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

func (b *Batch) encodeUint(n uint64) []byte {
	bites := make([]byte, 8)
	binary.BigEndian.PutUint64(bites[:], n)

	return bites[:]
}

func (b *Batch) WriteHeadNumber(n uint64) error {
	return b.add(HEAD, NUMBER, b.encodeUint(n))
}

func (b *Batch) write2(p, k []byte, v *fastrlp.Value) error {
	dst := v.MarshalTo(nil)

	return b.add(p, k, dst)
}
func (b *Batch) WriteReceipts(hash types.Hash, receipts []*types.Receipt) error {
	rr := types.Receipts(receipts)

	return b.writeRLP(RECEIPTS, hash.Bytes(), &rr)
}

// Write flushes any accumulated data to disk.
func (b *Batch) Write() error {
	return b.DB.Write(b.B, nil)
}

func (b *Batch) add(p []byte, k []byte, data []byte) error {
	p = append(p, k...)
	b.B.Put(p, data)

	b.Size += len(p) + len(data)

	return nil
}

func (b *Batch) writeRLP(p, k []byte, raw types.RLPMarshaler) error {
	var data []byte
	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	return b.add(p, k, data)
}

func (b *Batch) WriteCanonicalHeader(h *types.Header, diff *big.Int) error {
	if err := b.WriteHeader(h); err != nil {
		return err
	}

	if err := b.WriteHeadHash(h.Hash); err != nil {
		return err
	}

	if err := b.WriteHeadNumber(h.Number); err != nil {
		return err
	}

	if err := b.WriteCanonicalHash(h.Number, h.Hash); err != nil {
		return err
	}

	if err := b.WriteTotalDifficulty(h.Hash, diff); err != nil {
		return err
	}

	return nil
}

func (b *Batch) WriteCanonicalHash(n uint64, hash types.Hash) error {
	return b.add(CANONICAL, b.encodeUint(n), hash.Bytes())
}

func (b *Batch) WriteTotalDifficulty(hash types.Hash, diff *big.Int) error {
	return b.add(DIFFICULTY, hash.Bytes(), diff.Bytes())
}
