package types

import (
	goHex "encoding/hex"
	"strings"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

type ReceiptStatus uint64

const (
	ReceiptFailed ReceiptStatus = iota
	ReceiptSuccess
)

type Receipts []*Receipt

type Receipt struct {
	// consensus fields
	Root              Hash
	CumulativeGasUsed uint64
	LogsBloom         Bloom
	Logs              []*Log
	Status            *ReceiptStatus

	// context fields
	GasUsed         uint64
	ContractAddress *Address
	TxHash          Hash

	TransactionType TxType
}

func (r *Receipt) IsLegacyTx() bool {
	return r.TransactionType == LegacyTx
}

func (r *Receipt) SetStatus(s ReceiptStatus) {
	r.Status = &s
}

func (r *Receipt) SetContractAddress(contractAddress Address) {
	r.ContractAddress = &contractAddress
}

type Log struct {
	Address Address
	Topics  []Hash
	Data    []byte
}

const BloomByteLength = 256

type Bloom [BloomByteLength]byte

func (b *Bloom) UnmarshalText(input []byte) error {
	input = []byte(strings.TrimPrefix(strings.ToLower(string(input)), "0x"))
	if _, err := goHex.Decode(b[:], input); err != nil {
		return err
	}

	return nil
}

func (b Bloom) String() string {
	return hex.EncodeToHex(b[:])
}

// MarshalText implements encoding.TextMarshaler
func (b Bloom) MarshalText() ([]byte, error) {
	return []byte(b.String()), nil
}

// CreateBloom creates a new bloom filter from a set of receipts
func CreateBloom(receipts []*Receipt) (b Bloom) {
	h := keccak.DefaultKeccakPool.Get()
	defer keccak.DefaultKeccakPool.Put(h)

	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			b.setEncode(h, log.Address[:])

			for _, topic := range log.Topics {
				b.setEncode(h, topic[:])
			}
		}
	}

	return
}

func (b *Bloom) setEncode(hasher *keccak.Keccak, h []byte) {
	hasher.Reset()
	hasher.Write(h[:]) //nolint:errcheck
	buf := hasher.Read()

	for i := 0; i < 6; i += 2 {
		// Find the global bit location
		bit := (uint(buf[i+1]) + (uint(buf[i]) << 8)) & (BloomByteLength*8 - 1)

		// Find where the bit maps in the [0..BloomByteLength-1] byte array
		byteLocation := BloomByteLength - 1 - bit/8
		bitLocation := bit % 8
		b[byteLocation] |= 1 << bitLocation
	}
}

// IsLogInBloom checks if the log has a possible presence in the bloom filter
func (b *Bloom) IsLogInBloom(log *Log) bool {
	hasher := keccak.DefaultKeccakPool.Get()
	defer keccak.DefaultKeccakPool.Put(hasher)

	// Check if the log address is present
	if !b.isByteArrPresent(hasher, log.Address.Bytes()) {
		return false
	}

	// Check if all the topics are present
	for _, topic := range log.Topics {
		if !b.isByteArrPresent(hasher, topic.Bytes()) {
			return false
		}
	}

	return true
}

// isByteArrPresent checks if the byte array is possibly present in the Bloom filter
func (b *Bloom) isByteArrPresent(hasher *keccak.Keccak, data []byte) bool {
	hasher.Reset()
	hasher.Write(data[:]) //nolint:errcheck
	buf := hasher.Read()

	for i := 0; i < 6; i += 2 {
		// Find the global bit location
		bit := (uint(buf[i+1]) + (uint(buf[i]) << 8)) & (BloomByteLength*8 - 1)

		// Find where the bit maps in the [0..BloomByteLength-1] byte array
		byteLocation := BloomByteLength - 1 - bit/8
		bitLocation := bit % 8
		isSet := b[byteLocation] & (1 << bitLocation)

		if isSet == 0 {
			return false
		}
	}

	return true
}
