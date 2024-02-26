package storagev2

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical block
func (s *Storage) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	data, ok := s.get(CANONICAL, common.EncodeUint64ToBytes(n))
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *Storage) ReadHeadHash() (types.Hash, bool) {
	data, ok := s.get(HEAD_HASH, HEAD_HASH_KEY)
	if !ok {
		return types.Hash{}, false
	}

	return types.BytesToHash(data), true
}

// ReadHeadNumber returns the number of the head
func (s *Storage) ReadHeadNumber() (uint64, bool) {
	data, ok := s.get(HEAD_NUMBER, HEAD_NUMBER_KEY)
	if !ok {
		return 0, false
	}

	if len(data) != 8 {
		return 0, false
	}

	return common.EncodeBytesToUint64(data), true
}

// FORK //

// ReadForks read the current forks
func (s *Storage) ReadForks() ([]types.Hash, error) {
	forks := &Forks{}
	err := s.readRLP(FORK, FORK_KEY, forks)

	return *forks, err
}

// DIFFICULTY //

// ReadTotalDifficulty reads the difficulty
func (s *Storage) ReadTotalDifficulty(bn uint64, bh types.Hash) (*big.Int, bool) {
	v, ok := s.get(DIFFICULTY, getKey(bn, bh))
	if !ok {
		return nil, false
	}

	return big.NewInt(0).SetBytes(v), true
}

// HEADER //

// ReadHeader reads the header
func (s *Storage) ReadHeader(bn uint64, bh types.Hash) (*types.Header, error) {
	header := &types.Header{}
	err := s.readRLP(HEADER, getKey(bn, bh), header)

	return header, err
}

// BODY //

// ReadBody reads the body
func (s *Storage) ReadBody(bn uint64, bh types.Hash) (*types.Body, error) {
	body := &types.Body{}
	if err := s.readRLP(BODY, getKey(bn, bh), body); err != nil {
		return nil, err
	}

	// must read header because block number is needed in order to calculate each tx hash
	header := &types.Header{}
	if err := s.readRLP(HEADER, getKey(bn, bh), header); err != nil {
		return nil, err
	}

	for _, tx := range body.Transactions {
		tx.ComputeHash()
	}

	return body, nil
}

// RECEIPTS //

// ReadReceipts reads the receipts
func (s *Storage) ReadReceipts(bn uint64, bh types.Hash) ([]*types.Receipt, error) {
	receipts := &types.Receipts{}
	err := s.readRLP(RECEIPTS, getKey(bn, bh), receipts)

	return *receipts, err
}

// TX LOOKUP //

// ReadTxLookup reads the block number using the transaction hash
func (s *Storage) ReadTxLookup(hash types.Hash) (uint64, error) {
	return s.readLookup(TX_LOOKUP, hash)
}

// BLOCK LOOKUP //

// ReadBlockLookup reads the block number using the block hash
func (s *Storage) ReadBlockLookup(hash types.Hash) (uint64, error) {
	return s.readLookup(BLOCK_LOOKUP, hash)
}

func (s *Storage) readLookup(t uint8, hash types.Hash) (uint64, error) {
	data, ok := s.get(t, hash.Bytes())
	if !ok {
		return 0, ErrNotFound
	}

	if len(data) != 8 {
		return 0, ErrInvalidData
	}

	return common.EncodeBytesToUint64(data), nil
}

func (s *Storage) readRLP(t uint8, k []byte, raw types.RLPUnmarshaler) error {
	data, ok, err := s.getDB(t).Get(t, k)

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

func (s *Storage) get(t uint8, k []byte) ([]byte, bool) {
	data, ok, err := s.getDB(t).Get(t, k)

	if err != nil {
		return nil, false
	}

	return data, ok
}

func (s *Storage) getDB(t uint8) Database {
	i := getIndex(t)
	if s.db[i] != nil {
		return s.db[i]
	}

	return s.db[MAINDB_INDEX]
}
