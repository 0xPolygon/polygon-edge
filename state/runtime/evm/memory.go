package evm

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
)

type Memory struct {
	c           *Contract
	store       []byte
	lastGasCost uint64
}

func newMemory(c *Contract) *Memory {
	return &Memory{c: c, store: []byte{}, lastGasCost: 0}
}

func (m *Memory) Len() int {
	return len(m.store)
}

func roundUpToWord(n uint64) uint64 {
	return (n + 31) / 32 * 32
}

func numWords(n uint64) uint64 {
	if n > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (n + 31) / 32
}

func (m *Memory) Resize(size uint64) (uint64, error) {
	fee := uint64(0)

	if uint64(len(m.store)) < size {

		// expand in slots of 32 bytes
		words := numWords(size)
		size = roundUpToWord(size)

		// measure gas
		square := words * words
		linCoef := words * MemoryGas
		quadCoef := square / QuadCoeffDiv
		newTotalFee := linCoef + quadCoef

		fee = newTotalFee - m.lastGasCost

		if fee > m.c.gas {
			return 0, ErrGasConsumed
		}

		m.lastGasCost = newTotalFee

		m.store = append(m.store, make([]byte, size-uint64(len(m.store)))...)
	}

	return fee, nil
}

// calculates the memory size required for a step
func calcMemSize(off, l *big.Int) *big.Int {
	if l.Sign() == 0 {
		return common.Big0
	}

	return new(big.Int).Add(off, l)
}

func (m *Memory) SetByte(o *big.Int, val int64) (uint64, error) {
	offset := o.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, big.NewInt(1)))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}
	m.store[offset] = byte(val & 0xff)
	return gas, nil
}

func (m *Memory) Set32(o *big.Int, val *big.Int) (uint64, error) {
	offset := o.Uint64()

	size, overflow := bigUint64(big.NewInt(1).Add(o, big.NewInt(32)))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}

	copy(m.store[offset:offset+32], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	math.ReadBits(val, m.store[offset:offset+32])
	return gas, nil
}

func (m *Memory) Set(o *big.Int, l *big.Int, data []byte) (uint64, error) {
	offset := o.Uint64()
	length := l.Uint64()

	// length is zero
	if l.Sign() == 0 {
		return 0, nil
	}

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return 0, err
	}

	copy(m.store[offset:offset+length], data)
	return gas, nil
}

func (m *Memory) Get(o *big.Int, l *big.Int) ([]byte, uint64, error) {
	offset := o.Uint64()
	length := l.Uint64()

	if length == 0 {
		return []byte{}, 0, nil
	}

	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return nil, 0, ErrMemoryOverflow
	}

	gas, err := m.Resize(size)
	if err != nil {
		return nil, 0, err
	}

	cpy := make([]byte, length)
	copy(cpy, m.store[offset:offset+length])
	return cpy, gas, nil
}

func (m *Memory) Resize2(o *big.Int, l *big.Int) (uint64, error) {
	size, overflow := bigUint64(big.NewInt(1).Add(o, l))
	if overflow {
		return 0, ErrMemoryOverflow
	}

	return m.Resize(size)
}

func (m *Memory) Show() string {
	str := []string{}
	for i := 0; i < len(m.store); i += 16 {
		j := i + 16
		if j > len(m.store) {
			j = len(m.store)
		}

		str = append(str, hexutil.Encode(m.store[i:j]))
	}
	return strings.Join(str, "\n")
}
