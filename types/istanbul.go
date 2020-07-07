package types

import (
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/rlp"
)

var (
	// IstanbulDigest represents a hash of "Istanbul practical byzantine fault tolerance"
	// to identify whether the block is from Istanbul consensus engine
	IstanbulDigest = StringToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	IstanbulExtraVanity = 32 // Fixed number of extra-data bytes reserved for validator vanity
	IstanbulExtraSeal   = 65 // Fixed number of extra-data bytes reserved for validator seal

	// ErrInvalidIstanbulHeaderExtra is returned if the length of extra-data is less than 32 bytes
	ErrInvalidIstanbulHeaderExtra = errors.New("invalid istanbul header extra-data")
)

type HeaderWithoutHash struct {
	ParentHash   Hash     `json:"parentHash" db:"parent_hash"`
	Sha3Uncles   Hash     `json:"sha3Uncles" db:"sha3_uncles"`
	Miner        Address  `json:"miner" db:"miner"`
	StateRoot    Hash     `json:"stateRoot" db:"state_root"`
	TxRoot       Hash     `json:"transactionsRoot" db:"transactions_root"`
	ReceiptsRoot Hash     `json:"receiptsRoot" db:"receipts_root"`
	LogsBloom    Bloom    `json:"logsBloom" db:"logs_bloom"`
	Difficulty   uint64   `json:"difficulty" db:"difficulty"`
	Number       uint64   `json:"number" db:"number"`
	GasLimit     uint64   `json:"gasLimit" db:"gas_limit"`
	GasUsed      uint64   `json:"gasUsed" db:"gas_used"`
	Timestamp    uint64   `json:"timestamp" db:"timestamp"`
	ExtraData    HexBytes `json:"extraData" db:"extradata"`
	MixHash      Hash     `json:"mixHash" db:"mixhash"`
	Nonce        Nonce    `json:"nonce" db:"nonce"`
}

type IstanbulExtra struct {
	Validators    []Address
	Seal          []byte
	CommittedSeal [][]byte
}

// EncodeRLP serializes ist into the Ethereum RLP format.
func (ist *IstanbulExtra) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, []interface{}{
		ist.Validators,
		ist.Seal,
		ist.CommittedSeal,
	})
}

// DecodeRLP implements rlp.Decoder, and load the istanbul fields from a RLP stream.
func (ist *IstanbulExtra) DecodeRLP(s *rlp.Stream) error {
	var istanbulExtra struct {
		Validators    []Address
		Seal          []byte
		CommittedSeal [][]byte
	}
	if err := s.Decode(&istanbulExtra); err != nil {
		return err
	}
	ist.Validators, ist.Seal, ist.CommittedSeal = istanbulExtra.Validators, istanbulExtra.Seal, istanbulExtra.CommittedSeal
	return nil
}

// ExtractIstanbulExtra extracts all values of the IstanbulExtra from the header. It returns an
// error if the length of the given extra-data is less than 32 bytes or the extra-data can not
// be decoded.
func ExtractIstanbulExtra(h *Header) (*IstanbulExtra, error) {
	if len(h.ExtraData) < IstanbulExtraVanity {
		return nil, ErrInvalidIstanbulHeaderExtra
	}

	var istanbulExtra *IstanbulExtra
	err := rlp.DecodeBytes(h.ExtraData[IstanbulExtraVanity:], &istanbulExtra)
	if err != nil {
		return nil, err
	}
	return istanbulExtra, nil
}

// IstanbulFilteredHeader returns a filtered header which some information (like seal, committed seals)
// are clean to fulfill the Istanbul hash rules. It returns nil if the extra-data cannot be
// decoded/encoded by rlp.
func IstanbulFilteredHeader(h *Header, keepSeal bool) *HeaderWithoutHash {
	newHeader := h.Copy()
	istanbulExtra, err := ExtractIstanbulExtra(newHeader)
	if err != nil {
		return nil
	}

	if !keepSeal {
		istanbulExtra.Seal = []byte{}
	}
	istanbulExtra.CommittedSeal = [][]byte{}

	payload, err := rlp.EncodeToBytes(&istanbulExtra)
	if err != nil {
		return nil
	}

	newHeader.ExtraData = append(newHeader.ExtraData[:IstanbulExtraVanity], payload...)

	return &HeaderWithoutHash{
		ParentHash:   newHeader.ParentHash,
		Sha3Uncles:   newHeader.Sha3Uncles,
		Miner:        newHeader.Miner,
		StateRoot:    newHeader.StateRoot,
		TxRoot:       newHeader.TxRoot,
		ReceiptsRoot: newHeader.ReceiptsRoot,
		LogsBloom:    newHeader.LogsBloom,
		Difficulty:   newHeader.Difficulty,
		Number:       newHeader.Number,
		GasLimit:     newHeader.GasLimit,
		GasUsed:      newHeader.GasUsed,
		Timestamp:    newHeader.Timestamp,
		ExtraData:    newHeader.ExtraData,
		MixHash:      newHeader.MixHash,
		Nonce:        newHeader.Nonce,
	}
}
