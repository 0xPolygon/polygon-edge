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
	Address     Address  `json:"address"`
	Topics      []Hash   `json:"topics"`
	Data        HexBytes `json:"data"`
	BlockNumber uint64   `json:"blockNumber"`
	TxHash      Hash     `json:"transactionHash"`
	TxIndex     uint     `json:"transactionIndex"`
	BlockHash   Hash     `json:"blockHash"`
	LogIndex    uint     `json:"logIndex"`
	Removed     bool     `json:"removed"`
}

const BloomByteLength = 256

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
		bit := (uint(buf[i+1]) + (uint(buf[i]) << 8)) & 2047

		i := 256 - 1 - bit/8
		j := bit % 8
		b[i] = b[i] | (1 << j)
	}
}
