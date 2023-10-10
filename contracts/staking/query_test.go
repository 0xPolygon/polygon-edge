package staking

import (
	"errors"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/contracts/abis"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	addr1 = types.StringToAddress("1")
	addr2 = types.StringToAddress("2")
)

func leftPad(buf []byte, n int) []byte {
	l := len(buf)
	if l > n {
		return buf
	}

	tmp := make([]byte, n)
	copy(tmp[n-l:], buf)

	return tmp
}

func appendAll(bytesArrays ...[]byte) []byte {
	var res []byte

	for idx := range bytesArrays {
		res = append(res, bytesArrays[idx]...)
	}

	return res
}

type TxMock struct {
	hashToRes  map[types.Hash]*runtime.ExecutionResult
	nonce      map[types.Address]uint64
	nonPayable bool
}

func (m *TxMock) Apply(tx *types.Transaction) (*runtime.ExecutionResult, error) {
	if m.hashToRes == nil {
		return nil, nil
	}

	tx.ComputeHash(1)

	res, ok := m.hashToRes[tx.Hash]
	if ok {
		return res, nil
	}

	return nil, errors.New("not found")
}

func (m *TxMock) GetNonce(addr types.Address) uint64 {
	if m.nonce != nil {
		return m.nonce[addr]
	}

	return 0
}

func (m *TxMock) SetNonPayable(nonPayable bool) {
	m.nonPayable = nonPayable
}

func Test_decodeValidators(t *testing.T) {
	tests := []struct {
		name     string
		value    []byte
		succeed  bool
		expected []types.Address
	}{
		{
			name: "should fail to parse",
			value: appendAll(
				leftPad([]byte{0x20}, 32), // Offset of the beginning of array
				leftPad([]byte{0x01}, 32), // Number of addresses
			),
			succeed: false,
		},
		{
			name: "should succeed",
			value: appendAll(
				leftPad([]byte{0x20}, 32), // Offset of the beginning of array
				leftPad([]byte{0x02}, 32), // Number of addresses
				leftPad(addr1[:], 32),     // Address 1
				leftPad(addr2[:], 32),     // Address 2
			),
			succeed: true,
			expected: []types.Address{
				addr1,
				addr2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := abis.StakingABI.Methods["validators"]
			assert.NotNil(t, method)

			res, err := DecodeValidators(method, tt.value)
			if tt.succeed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expected, res)
		})
	}
}

func TestQueryValidators(t *testing.T) {
	method := abis.StakingABI.Methods["validators"]
	if method == nil {
		t.Fail()
	}

	type MockArgs struct {
		addr types.Address
		tx   *types.Transaction
	}

	type MockReturns struct {
		nonce uint64
		res   *runtime.ExecutionResult
		err   error
	}

	tests := []struct {
		name        string
		from        types.Address
		mockArgs    *MockArgs
		mockReturns *MockReturns
		succeed     bool
		expected    []types.Address
		err         error
	}{
		{
			name: "should failed",
			from: addr1,
			mockArgs: &MockArgs{
				addr: addr1,
				tx: &types.Transaction{
					From:     addr1,
					To:       &AddrStakingContract,
					Value:    big.NewInt(0),
					Input:    method.ID(),
					GasPrice: big.NewInt(0),
					Gas:      100000000,
					Nonce:    10,
				},
			},
			mockReturns: &MockReturns{
				nonce: 10,
				res: &runtime.ExecutionResult{
					Err: runtime.ErrExecutionReverted,
				},
				err: nil,
			},
			succeed:  false,
			expected: nil,
			err:      runtime.ErrExecutionReverted,
		},
		{
			name: "should succeed",
			from: addr1,
			mockArgs: &MockArgs{
				addr: addr1,
				tx: &types.Transaction{
					From:     addr1,
					To:       &AddrStakingContract,
					Value:    big.NewInt(0),
					Input:    method.ID(),
					GasPrice: big.NewInt(0),
					Gas:      queryGasLimit,
					Nonce:    10,
				},
			},
			mockReturns: &MockReturns{
				nonce: 10,
				res: &runtime.ExecutionResult{
					ReturnValue: appendAll(
						leftPad([]byte{0x20}, 32), // Offset of the beginning of array
						leftPad([]byte{0x00}, 32), // Number of addresses
					),
				},
				err: nil,
			},
			succeed:  true,
			expected: []types.Address{},
			err:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := abis.StakingABI.Methods["validators"]
			assert.NotNil(t, method)

			mock := &TxMock{
				hashToRes: map[types.Hash]*runtime.ExecutionResult{
					tt.mockArgs.tx.ComputeHash(1).Hash: tt.mockReturns.res,
				},
				nonce: map[types.Address]uint64{
					tt.mockArgs.addr: tt.mockReturns.nonce,
				},
			}

			res, err := QueryValidators(mock, tt.from)
			if tt.succeed {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expected, res)
		})
	}
}
