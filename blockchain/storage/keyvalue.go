//nolint:stylecheck
package storage

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
)

// Prefixes for the key-value store
var (
	// DIFFICULTY is the difficulty prefix
	DIFFICULTY = []byte("d")

	// HEADER is the header prefix
	HEADER = []byte("h")

	// HEAD is the chain head prefix
	HEAD = []byte("o")

	// FORK is the entry to store forks
	FORK = []byte("f")

	// CANONICAL is the prefix for the canonical chain numbers
	CANONICAL = []byte("c")

	// BODY is the prefix for bodies
	BODY = []byte("b")

	// RECEIPTS is the prefix for receipts
	RECEIPTS = []byte("r")

	// SNAPSHOTS is the prefix for snapshots
	SNAPSHOTS = []byte("s")

	// TX_LOOKUP_PREFIX is the prefix for transaction lookups
	TX_LOOKUP_PREFIX = []byte("l")
)

// Sub-prefixes
var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("empty")
)

// KV is a key value storage interface.
//
// KV = Key-Value
type KV interface {
	Close() error
	Set(p []byte, v []byte) error
	Get(p []byte) ([]byte, bool, error)
}

// KeyValueStorage is a generic storage for kv databases
type KeyValueStorage struct {
	logger hclog.Logger
	db     KV
	Db     KV
}

func NewKeyValueStorage(logger hclog.Logger, db KV) Storage {
	return &KeyValueStorage{logger: logger, db: db}
}

func (s *KeyValueStorage) encodeUint(n uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b[:], n)

	return b[:]
}

func (s *KeyValueStorage) decodeUint(b []byte) uint64 {
	return binary.BigEndian.Uint64(b[:])
}

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *KeyValueStorage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	data, ok := s.get(CANONICAL, s.encodeUint(n))
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// WriteCanonicalHash writes a hash for a number block in the canonical chain
func (s *KeyValueStorage) WriteCanonicalHash(n uint64, hash types.Hash) error {
	return s.set(CANONICAL, s.encodeUint(n), hash.Bytes())
}

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *KeyValueStorage) ReadHeadHash() (types.Hash, bool) {
	data, ok := s.get(HEAD, HASH)
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// ReadHeadNumber returns the number of the head
func (s *KeyValueStorage) ReadHeadNumber() (uint64, bool) {
	data, ok := s.get(HEAD, NUMBER)
	if !ok {
		return 0, false
	}

	if len(data) != 8 {
		return 0, false
	}

	return s.decodeUint(data), true
}

// WriteHeadHash writes the hash of the head
func (s *KeyValueStorage) WriteHeadHash(h types.Hash) error {
	return s.set(HEAD, HASH, h.Bytes())
}

// WriteHeadNumber writes the number of the head
func (s *KeyValueStorage) WriteHeadNumber(n uint64) error {
	return s.set(HEAD, NUMBER, s.encodeUint(n))
}

// FORK //

// WriteForks writes the current forks
func (s *KeyValueStorage) WriteForks(forks []types.Hash) error {
	ff := Forks(forks)

	return s.writeRLP(FORK, EMPTY, &ff)
}

// ReadForks read the current forks
func (s *KeyValueStorage) ReadForks() ([]types.Hash, error) {
	forks := &Forks{}
	err := s.readRLP(FORK, EMPTY, forks)

	return *forks, err
}

// DIFFICULTY //

// WriteTotalDifficulty writes the difficulty
func (s *KeyValueStorage) WriteTotalDifficulty(hash types.Hash, diff *big.Int) error {
	return s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadTotalDifficulty reads the difficulty
func (s *KeyValueStorage) ReadTotalDifficulty(hash types.Hash) (*big.Int, bool) {
	v, ok := s.get(DIFFICULTY, hash.Bytes())
	if !ok {
		return nil, false
	}

	return big.NewInt(0).SetBytes(v), true
}

// HEADER //

// WriteHeader writes the header
func (s *KeyValueStorage) WriteHeader(h *types.Header) error {
	return s.writeRLP(HEADER, h.Hash.Bytes(), h)
}

// ReadHeader reads the header
func (s *KeyValueStorage) ReadHeader(hash types.Hash) (*types.Header, error) {
	header := &types.Header{}
	err := s.readRLP(HEADER, hash.Bytes(), header)

	return header, err
}

// WriteCanonicalHeader implements the storage interface
func (s *KeyValueStorage) WriteCanonicalHeader(h *types.Header, diff *big.Int) error {
	if err := s.WriteHeader(h); err != nil {
		return err
	}

	if err := s.WriteHeadHash(h.Hash); err != nil {
		return err
	}

	if err := s.WriteHeadNumber(h.Number); err != nil {
		return err
	}

	if err := s.WriteCanonicalHash(h.Number, h.Hash); err != nil {
		return err
	}

	if err := s.WriteTotalDifficulty(h.Hash, diff); err != nil {
		return err
	}

	return nil
}

// BODY //

// WriteBody writes the body
func (s *KeyValueStorage) WriteBody(hash types.Hash, body *types.Body) error {
	return s.writeRLP(BODY, hash.Bytes(), body)
}

// ReadBody reads the body
func (s *KeyValueStorage) ReadBody(hash types.Hash) (*types.Body, error) {
	body := &types.Body{}
	err := s.readRLP(BODY, hash.Bytes(), body)

	return body, err
}

// RECEIPTS //

// WriteReceipts writes the receipts
func (s *KeyValueStorage) WriteReceipts(hash types.Hash, receipts []*types.Receipt) error {
	rr := types.Receipts(receipts)

	return s.writeRLP(RECEIPTS, hash.Bytes(), &rr)
}

// ReadReceipts reads the receipts
func (s *KeyValueStorage) ReadReceipts(hash types.Hash) ([]*types.Receipt, error) {
	receipts := &types.Receipts{}
	err := s.readRLP(RECEIPTS, hash.Bytes(), receipts)

	return *receipts, err
}

// TX LOOKUP //

// WriteTxLookup maps the transaction hash to the block hash
func (s *KeyValueStorage) WriteTxLookup(hash types.Hash, blockHash types.Hash) error {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes())

	return s.write2(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

// ReadTxLookup reads the block hash using the transaction hash
func (s *KeyValueStorage) ReadTxLookup(hash types.Hash) (types.Hash, bool) {
	parser := &fastrlp.Parser{}

	v := s.read2(TX_LOOKUP_PREFIX, hash.Bytes(), parser)
	if v == nil {
		return types.Hash{}, false
	}

	blockHash := []byte{}
	blockHash, err := v.GetBytes(blockHash[:0], 32)

	if err != nil {
		panic(err)
	}

	return types.BytesToHash(blockHash), true
}

// WRITE OPERATIONS //

func (s *KeyValueStorage) writeRLP(p, k []byte, raw types.RLPMarshaler) error {
	var data []byte
	if obj, ok := raw.(types.RLPStoreMarshaler); ok {
		data = obj.MarshalStoreRLPTo(nil)
	} else {
		data = raw.MarshalRLPTo(nil)
	}

	return s.set(p, k, data)
}

var ErrNotFound = fmt.Errorf("not found")

func (s *KeyValueStorage) readRLP(p, k []byte, raw types.RLPUnmarshaler) error {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return err
	}

	if !ok {
		return ErrNotFound
	}

	if obj, ok := raw.(types.RLPStoreUnmarshaler); ok {
		// decode in the store format
		if err := obj.UnmarshalStoreRLP(data); err != nil {
			return err
		}
	} else {
		// normal rlp decoding
		if err := raw.UnmarshalRLP(data); err != nil {
			return err
		}
	}

	return nil
}

func (s *KeyValueStorage) read2(p, k []byte, parser *fastrlp.Parser) *fastrlp.Value {
	data, ok := s.get(p, k)
	if !ok {
		return nil
	}

	v, err := parser.Parse(data)
	if err != nil {
		return nil
	}

	return v
}

func (s *KeyValueStorage) write2(p, k []byte, v *fastrlp.Value) error {
	dst := v.MarshalTo(nil)

	return s.set(p, k, dst)
}

func (s *KeyValueStorage) set(p []byte, k []byte, v []byte) error {
	p = append(p, k...)

	return s.db.Set(p, v)
}

func (s *KeyValueStorage) get(p []byte, k []byte) ([]byte, bool) {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)

	if err != nil {
		return nil, false
	}

	return data, ok
}

// Close closes the connection with the db
func (s *KeyValueStorage) Close() error {
	return s.db.Close()
}
