//nolint:stylecheck
package storagev2

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *Storage) ReadCanonicalHash(n uint64) (types.Hash, error) {
	data, err := s.get(CANONICAL, common.EncodeUint64ToBytes(n))
	if err != nil {
		return types.Hash{}, err
	}

	return types.BytesToHash(data), nil
}

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *Storage) ReadHeadHash() (types.Hash, error) {
	data, err := s.get(HEAD_HASH, HEAD_HASH_KEY)
	if err != nil {
		return types.Hash{}, err
	}

	return types.BytesToHash(data), nil
}

// ReadHeadNumber returns the number of the head
func (s *Storage) ReadHeadNumber() (uint64, error) {
	data, err := s.get(HEAD_NUMBER, HEAD_NUMBER_KEY)
	if err != nil {
		return 0, err
	}

	if len(data) != 8 {
		return 0, errors.New("Invalid data")
	}

	return common.EncodeBytesToUint64(data), nil
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
func (s *Storage) ReadTotalDifficulty(bn uint64) (*big.Int, error) {
	v, err := s.get(DIFFICULTY, common.EncodeUint64ToBytes(bn))
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).SetBytes(v), nil
}

// HEADER //

// ReadHeader reads the header
func (s *Storage) ReadHeader(bn uint64) (*types.Header, error) {
	header := &types.Header{}
	err := s.readRLP(HEADER, common.EncodeUint64ToBytes(bn), header)

	return header, err
}

// BODY //

// ReadBody reads the body
func (s *Storage) ReadBody(bn uint64) (*types.Body, error) {
	body := &types.Body{}
	if err := s.readRLP(BODY, common.EncodeUint64ToBytes(bn), body); err != nil {
		return nil, err
	}

	// must read header because block number is needed in order to calculate each tx hash
	header := &types.Header{}
	if err := s.readRLP(HEADER, common.EncodeUint64ToBytes(bn), header); err != nil {
		return nil, err
	}

	for _, tx := range body.Transactions {
		tx.ComputeHash()
	}

	return body, nil
}

// RECEIPTS //

// ReadReceipts reads the receipts
func (s *Storage) ReadReceipts(bn uint64) ([]*types.Receipt, error) {
	receipts := &types.Receipts{}
	err := s.readRLP(RECEIPTS, common.EncodeUint64ToBytes(bn), receipts)

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
	data, err := s.get(t, hash.Bytes())
	if err != nil {
		return 0, err
	}

	if len(data) != 8 {
		return 0, errors.New("Invalid data")
	}

	return common.EncodeBytesToUint64(data), nil
}

func (s *Storage) readRLP(t uint8, k []byte, raw types.RLPUnmarshaler) error {
	data, err := s.getDB(t).Get(t, k)

	if err != nil {
		return err
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

func (s *Storage) get(t uint8, k []byte) ([]byte, error) {
	data, err := s.getDB(t).Get(t, k)

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *Storage) getDB(t uint8) Database {
	i := getIndex(t)
	if s.db[i] != nil {
		return s.db[i]
	}

	return s.db[MAINDB_INDEX]
}
