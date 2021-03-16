package storage

import (
	"bytes"
	"encoding/binary"
	"math/big"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"

	"github.com/0xPolygon/minimal/types"
)

// prefix

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

	// TRANSACTION is the prefix for transactions
	TX_LOOKUP_PREFIX = []byte("l")

	// CHAIN_INDEXER is the prefix for the chain indexer
	CHAIN_INDEXER_HEAD = []byte("chainIndexerSectionsHead")
	CHAIN_INDEXER      = []byte("chainIndexer")
)

// sub-prefix

var (
	HASH   = []byte("hash")
	NUMBER = []byte("number")
	EMPTY  = []byte("empty")

	CHAIN_SECTIONS = []byte("sectionsValid")
)

// KV is a key value storage interface
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

// -- head --

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

// -- fork --

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

// -- difficulty --

// WriteDiff writes the difficulty
func (s *KeyValueStorage) WriteDiff(hash types.Hash, diff *big.Int) error {
	return s.set(DIFFICULTY, hash.Bytes(), diff.Bytes())
}

// ReadDiff reads the difficulty
func (s *KeyValueStorage) ReadDiff(hash types.Hash) (*big.Int, bool) {
	v, ok := s.get(DIFFICULTY, hash.Bytes())
	if !ok {
		return nil, false
	}
	return big.NewInt(0).SetBytes(v), true
}

// -- header --

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
	if err := s.WriteDiff(h.Hash, diff); err != nil {
		return err
	}
	return nil
}

// -- body --

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

// -- snapshots --

// WriteBody writes the body
func (s *KeyValueStorage) WriteSnapshot(hash types.Hash, blob []byte) error {
	return s.set(SNAPSHOTS, hash.Bytes(), blob)
}

// ReadBody reads the body
func (s *KeyValueStorage) ReadSnapshot(hash types.Hash) ([]byte, bool) {
	data, ok := s.get(SNAPSHOTS, hash.Bytes())
	if !ok {
		return []byte{}, false
	}
	return data, true
}

// -- receipts --

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

// -- tx lookup --

// WriteReceipts writes the receipts
func (s *KeyValueStorage) WriteTxLookup(hash types.Hash, blockHash types.Hash) error {
	ar := &fastrlp.Arena{}
	vr := ar.NewBytes(blockHash.Bytes())
	return s.write2(TX_LOOKUP_PREFIX, hash.Bytes(), vr)
}

// ReadReceipts reads the receipts
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

// -- write ops --

func (s *KeyValueStorage) writeRLP(p, k []byte, obj types.RLPMarshaler) error {
	return s.set(p, k, obj.MarshalRLPTo(nil))
}

func (s *KeyValueStorage) readRLP(p, k []byte, obj types.RLPUnmarshaler) error {
	p = append(p, k...)
	data, ok, err := s.db.Get(p)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := obj.UnmarshalRLP(data); err != nil {
		return err
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

// Prefix, Key, Value
func (s *KeyValueStorage) set(prefix []byte, key []byte, value []byte) error {
	prefix = append(prefix, key...)
	return s.db.Set(prefix, value)
}

func (s *KeyValueStorage) get(prefix []byte, key []byte) ([]byte, bool) {
	prefix = append(prefix, key...)
	data, ok, err := s.db.Get(prefix)
	if err != nil {
		return nil, false
	}
	return data, ok
}

// Chain indexer //

func (s *KeyValueStorage) ReadIndexSectionHead(section uint64) types.Hash {
	var sectionBinary []byte
	binary.BigEndian.PutUint64(sectionBinary[:], section)

	data, _ := s.get(CHAIN_INDEXER, sectionBinary)

	return types.BytesToHash(data)
}

func (s *KeyValueStorage) WriteIndexSectionHead(section uint64, hash types.Hash) error {
	var sectionBinary []byte
	binary.BigEndian.PutUint64(sectionBinary[:], section)

	err := s.set(CHAIN_INDEXER_HEAD, sectionBinary, hash.Bytes())

	return err
}

func (s *KeyValueStorage) RemoveSectionHead(section uint64) error {

	// TODO swap with remove
	var sectionBinary []byte
	binary.BigEndian.PutUint64(sectionBinary[:], section)

	err := s.set(CHAIN_INDEXER_HEAD, sectionBinary, nil)

	return err
}

func (s *KeyValueStorage) WriteValidSectionsNum(sections uint64) error {
	var sectionsBinary []byte
	binary.BigEndian.PutUint64(sectionsBinary[:], sections)

	err := s.set(CHAIN_INDEXER, CHAIN_SECTIONS, sectionsBinary)

	return err
}

func (s *KeyValueStorage) ReadValidSectionsNum() (uint64, error) {
	var ret uint64

	data, _ := s.get(CHAIN_INDEXER, CHAIN_SECTIONS)

	buf := bytes.NewBuffer(data)
	_ = binary.Read(buf, binary.BigEndian, &ret)

	return ret, nil
}

func (s *KeyValueStorage) WriteBloomBits(bitNum uint, currentSection uint64, bHead types.Hash, bits []byte) {
	var bitNumBinary []byte
	binary.BigEndian.PutUint64(bitNumBinary[:], uint64(bitNum))

	var currentSectionBinary []byte
	binary.BigEndian.PutUint64(currentSectionBinary[:], uint64(currentSection))

	generatedKey := append(bitNumBinary, currentSectionBinary...)

	_ = s.set(CHAIN_INDEXER, append(generatedKey, bHead.Bytes()...), bits)
}

// Close closes the connection with the db
func (s *KeyValueStorage) Close() error {
	return s.db.Close()
}
