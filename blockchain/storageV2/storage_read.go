//nolint:stylecheck
package storageV2

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
)

// -- canonical hash --

// ReadCanonicalHash gets the hash from the number of the canonical chain
func (s *Storage) ReadCanonicalHash(n uint64) (types.Hash, error) {
	data, err := s.get(common.EncodeUint64ToBytes(n), CANONICAL)
	if err != nil {
		return types.Hash{}, err
	}

	return types.BytesToHash(data), nil
}

// HEAD //

// ReadHeadHash returns the hash of the head
func (s *Storage) ReadHeadHash() (types.Hash, error) {
	data, err := s.get(HASH, GIDLID)
	if err != nil {
		return types.Hash{}, err
	}

	return types.BytesToHash(data), nil
}

// ReadHeadNumber returns the number of the head
func (s *Storage) ReadHeadNumber() (uint64, error) {
	data, err := s.get(NUMBER, GIDLID)
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
	err := s.readRLP(FORK, GIDLID, forks)

	return *forks, err
}

// DIFFICULTY //

// ReadTotalDifficulty reads the difficulty
func (s *Storage) ReadTotalDifficulty(bn uint64) (*big.Int, error) {
	v, err := s.get(common.EncodeUint64ToBytes(bn), DIFFICULTY)
	if err != nil {
		return nil, err
	}

	return big.NewInt(0).SetBytes(v), nil
}

// HEADER //

// ReadHeader reads the header
func (s *Storage) ReadHeader(bn uint64) (*types.Header, error) {
	header := &types.Header{}
	err := s.readRLP(common.EncodeUint64ToBytes(bn), HEADER, header)

	return header, err
}

// BODY //

// ReadBody reads the body
func (s *Storage) ReadBody(bn uint64) (*types.Body, error) {
	body := &types.Body{}
	if err := s.readRLP(common.EncodeUint64ToBytes(bn), BODY, body); err != nil {
		return nil, err
	}

	// must read header because block number is needed in order to calculate each tx hash
	header := &types.Header{}
	if err := s.readRLP(common.EncodeUint64ToBytes(bn), HEADER, header); err != nil {
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
	err := s.readRLP(common.EncodeUint64ToBytes(bn), RECEIPTS, receipts)

	return *receipts, err
}

// TX LOOKUP //

// ReadTxLookup reads the block number using the transaction hash
func (s *Storage) ReadTxLookup(hash types.Hash) (uint64, error) {
	return s.readLookup(hash)
}

// BLOCK LOOKUP //

// ReadBlockLookup reads the block number using the block hash
func (s *Storage) ReadBlockLookup(hash types.Hash) (uint64, error) {
	return s.readLookup(hash)
}

func (s *Storage) readLookup(hash types.Hash) (uint64, error) {
	data, err := s.get(hash.Bytes(), GIDLID)
	if err != nil {
		return 0, err
	}

	if len(data) != 8 {
		return 0, errors.New("Invalid data")
	}

	return common.EncodeBytesToUint64(data), nil
}

func (s *Storage) readRLP(k, mc []byte, raw types.RLPUnmarshaler) error {
	k = append(k, mc...)
	data, err := s.getDB(mc).Get(k)

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

func (s *Storage) get(k, mc []byte) ([]byte, error) {
	k = append(k, mc...)
	data, err := s.getDB(mc).Get(k)

	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *Storage) getDB(mc []byte) Database {
	i := getIndex(mc)
	if s.db[i] != nil {
		return s.db[i]
	}

	return s.db[MAINDB_INDEX]
}
