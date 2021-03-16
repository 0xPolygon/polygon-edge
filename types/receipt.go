package types

import (
	"database/sql/driver"

	goHex "encoding/hex"

	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
)

type ReceiptStatus uint64

const (
	ReceiptFailed ReceiptStatus = iota
	ReceiptSuccess
)

type Receipts []*Receipt

type Receipt struct {
	Root              Hash           `json:"root" db:"root"`
	CumulativeGasUsed uint64         `json:"cumulativeGasUsed" db:"cumulative_gas_used"`
	LogsBloom         Bloom          `json:"logsBloom" db:"bloom"`
	Logs              []*Log         `json:"logs"`
	Status            *ReceiptStatus `json:"status"`
	TxHash            Hash           `json:"transactionHash" db:"txhash"`
	ContractAddress   Address        `json:"contractAddress" db:"contract_address"`
	GasUsed           uint64         `json:"gasUsed" db:"gas_used"`
}

func (r *Receipt) SetStatus(s ReceiptStatus) {
	r.Status = &s
}

type Log struct {
	Address Address  `json:"address"`
	Topics  []Hash   `json:"topics"`
	Data    HexBytes `json:"data"`
}

const BloomByteLength = 256
const BloomBitLength = 256 * 8

type Bloom [BloomByteLength]byte

func (b *Bloom) UnmarshalText(input []byte) error {
	input = input[2:]
	if _, err := goHex.Decode(b[:], input); err != nil {
		return err
	}
	return nil
}

func (b Bloom) String() string {
	return hex.EncodeToHex(b[:])
}

func (b Bloom) Value() (driver.Value, error) {
	return b.String(), nil
}

func (b *Bloom) Scan(src interface{}) error {
	bb := hex.MustDecodeHex(string(src.([]byte)))
	copy(b[:], bb[:])
	return nil
}

// CreateBloom creates a new bloom filter from a set of receipts
func CreateBloom(receipts []*Receipt) (b Bloom) {
	h := keccak.DefaultKeccakPool.Get()
	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			b.setEncode(h, log.Address[:])
			for _, topic := range log.Topics {
				b.setEncode(h, topic[:])
			}
		}
	}
	keccak.DefaultKeccakPool.Put(h)
	return
}

func (b *Bloom) setEncode(hasher *keccak.Keccak, h []byte) {
	hasher.Reset()
	hasher.Write(h[:])
	buf := hasher.Read()

	for i := 0; i < 6; i += 2 {
		// Find the global bit location
		bit := (uint(buf[i+1]) + (uint(buf[i]) << 8)) & 2047

		// Find where the bit maps in the [0..255] byte array
		byteLocation := 256 - 1 - bit/8
		bitLocation := bit % 8
		b[byteLocation] = b[byteLocation] | (1 << bitLocation)
	}
}

// IsLogInBloom checks if the log has a possible presence in the bloom filter
func (b *Bloom) IsLogInBloom(log *Log) bool {
	hasher := keccak.DefaultKeccakPool.Get()

	// Check if the log address is present
	addressPresent := b.isByteArrPresent(hasher, log.Address.Bytes())
	if !addressPresent {
		return false
	}

	// Check if all the topics are present
	for _, topic := range log.Topics {
		topicsPresent := b.isByteArrPresent(hasher, topic.Bytes())

		if !topicsPresent {
			return false
		}
	}
	
	keccak.DefaultKeccakPool.Put(hasher)

	return true
}

// isByteArrPresent checks if the byte array is possibly present in the Bloom filter
func (b *Bloom) isByteArrPresent(hasher *keccak.Keccak, data []byte) bool {
	hasher.Reset()
	hasher.Write(data[:])
	buf := hasher.Read()

	for i := 0; i < 6; i += 2 {
		// Find the global bit location
		bit := (uint(buf[i+1]) + (uint(buf[i]) << 8)) & 2047

		// Find where the bit maps in the [0..255] byte array
		byteLocation := 256 - 1 - bit/8
		bitLocation := bit % 8

		referenceByte := b[byteLocation]
		
		isSet := int(referenceByte & (1 << (bitLocation - 1)))

		if isSet == 0 {
			return false
		}
	}

	return true
}
