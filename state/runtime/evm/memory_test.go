package evm

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func expectLength(t *testing.T, m *Memory, len int) {
	if m.Len() != len {
		t.Fatalf("expected length %d but found %d", len, m.Len())
	}
}

func TestMemorySetResize(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))
	data := mustDecode("0x123456")

	m.Resize(0 + 3)
	m.Set(0, 3, data)
	expectLength(t, m, 32)

	equalBytes(t, m.store, common.RightPadBytes(data, 32))
	found, _, err := m.Get(c(0), c(3))
	if err != nil {
		t.Fatal(err)
	}
	equalBytes(t, found, data)

	// resize not necessary
	m.Set(10, 3, data)
	expectLength(t, m, 32)

	m.Resize(65 + 10)
	m.Set(65, 10, data)
	expectLength(t, m, 96)

	// take two more slots
	m.Resize(129 + 65)
	m.Set(129, 65, data)
	expectLength(t, m, 224)
}

func TestMemorySetByte(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))

	m.SetByte(c(10), 10)
	expectLength(t, m, 32)

	m.SetByte(c(31), 10)
	expectLength(t, m, 32)

	m.SetByte(c(32), 10)
	expectLength(t, m, 64)
}

func TestMemorySet32(t *testing.T) {
	m := newMemory(newTestContract([]byte{}))

	m.Set32(c(0), big.NewInt(32))
	expectLength(t, m, 32)

	m.Set32(c(1), big.NewInt(32))
	expectLength(t, m, 64)

	m = newMemory(newTestContract([]byte{}))
	m.Set32(c(0), big.NewInt(32))
	expectLength(t, m, 32)

	m.Set32(c(32), big.NewInt(32))
	expectLength(t, m, 64)
}
