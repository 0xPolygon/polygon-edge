package ethereum

import (
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
)

// hashOrNumber is a combined field for specifying an origin block.
type hashOrNumber struct {
	Hash   common.Hash
	Number uint64
}

// EncodeRLP encodes hashOrNumber to RLP format
func (h *hashOrNumber) EncodeRLP(w io.Writer) error {
	if h.Hash == (common.Hash{}) {
		return rlp.Encode(w, h.Number)
	}
	if h.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", h.Hash, h.Number)
	}
	return rlp.Encode(w, h.Hash)
}

// IsHash checks if its a hash
func (h *hashOrNumber) IsHash() bool {
	return h.Hash != (common.Hash{})
}

// DecodeRLP decodes hashOrNumber in RLP to object
func (h *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &h.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &h.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}
