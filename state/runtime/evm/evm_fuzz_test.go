package evm

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	go_fuzz_utils "github.com/trailofbits/go-fuzz-utils"
)

var _ runtime.Host = &mockHostF{}

// mockHostF is a struct which meets the requirements of runtime.Host interface but returns naive data
type mockHostF struct {
	// to use
	tracer   runtime.VMTracer
	storage  map[types.Address]map[types.Hash]types.Hash
	balances map[types.Address]*big.Int
	nonces   map[types.Address]uint64

	// to fuzz
	refund    uint64
	blockHash types.Hash
}

func (m *mockHostF) AccountExists(addr types.Address) bool {
	if _, ok := m.nonces[addr]; ok {
		return true
	}

	return false
}

func (m *mockHostF) GetStorage(addr types.Address, key types.Hash) types.Hash {
	if val, ok := m.storage[addr][key]; !ok {
		return types.Hash{}
	} else {
		return val
	}
}

func (m *mockHostF) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
	if _, ok := m.storage[addr]; !ok {
		m.storage[addr] = make(map[types.Hash]types.Hash)
	}

	m.storage[addr][key] = value

	return runtime.StorageModified
}

func (m *mockHostF) SetState(addr types.Address, key types.Hash, value types.Hash) {
	return
}

func (m *mockHostF) SetNonPayable(nonPayable bool) {
	return
}

func (m *mockHostF) GetBalance(addr types.Address) *big.Int {
	if b, ok := m.balances[addr]; !ok {
		m.balances[addr] = big.NewInt(0)

		return m.balances[addr]
	} else {
		return b
	}
}

func (m *mockHostF) GetCodeSize(addr types.Address) int {
	return 0
}

func (m *mockHostF) GetCodeHash(addr types.Address) types.Hash {
	return types.Hash{}
}

func (m *mockHostF) GetCode(addr types.Address) []byte {
	return nil
}

func (m *mockHostF) Selfdestruct(addr types.Address, beneficiary types.Address) {
	return
}
func (m *mockHostF) GetTxContext() runtime.TxContext {
	return runtime.TxContext{}
}

func (m *mockHostF) GetBlockHash(number int64) types.Hash {
	return m.blockHash
}

func (m *mockHostF) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	return
}

func (m *mockHostF) Callx(c *runtime.Contract, h runtime.Host) *runtime.ExecutionResult {
	return &runtime.ExecutionResult{}
}

func (m *mockHostF) Empty(addr types.Address) bool {
	return true
}

func (m *mockHostF) GetNonce(addr types.Address) uint64 {
	if nonce, ok := m.nonces[addr]; !ok {
		m.nonces[addr] = 0

		return 0
	} else {
		return nonce
	}
}

func (m *mockHostF) Transfer(from types.Address, to types.Address, amount *big.Int) error {
	f := m.GetBalance(from)
	t := m.GetBalance(to)

	if f.Cmp(amount) < 0 {
		return errors.New("not enough balance")
	}

	f.Sub(f, amount)
	t.Add(t, amount)

	return nil
}

func (m *mockHostF) GetTracer() runtime.VMTracer {
	return m.tracer
}

func (m *mockHostF) GetRefund() uint64 {
	return m.refund
}

func FuzzTestEVM(f *testing.F) {
	seed := []byte{
		PUSH1, 0x01, PUSH1, 0x02, ADD,
		PUSH1, 0x00, MSTORE8,
		PUSH1, 0x01, PUSH1, 0x00, RETURN,
	}

	f.Add(seed)

	config := &chain.ForksInTime{
		Byzantium: true,
	}

	evm := NewEVM()

	f.Fuzz(func(t *testing.T, input []byte) {
		tp, err := go_fuzz_utils.NewTypeProvider(input)
		if err != nil {
			return
		}

		err = tp.SetParamsSliceBounds(1, 4*1024)
		if err != nil {
			return
		}

		refund, err := tp.GetUint64()
		if err != nil {
			return
		}

		blockHashI, err := tp.GetNBytes(types.HashLength)
		if err != nil {
			return
		}

		blockHash := types.BytesToHash(blockHashI)
		host := &mockHostF{
			refund: refund, blockHash: blockHash,
			storage:  make(map[types.Address]map[types.Hash]types.Hash),
			balances: make(map[types.Address]*big.Int),
			nonces:   make(map[types.Address]uint64),
		}

		code, err := tp.GetBytes()
		if err != nil {
			return
		}

		contract := newMockContract(big.NewInt(0), 10000000, code)
		evm.Run(contract, host, config)
	})
}
