package evm

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	two   = big.NewInt(2)
	three = big.NewInt(3)
	four  = big.NewInt(4)
	five  = big.NewInt(5)

	allEnabledForks = chain.AllForksEnabled.At(0)
)

type OperandsLogical struct {
	operands       []*big.Int
	expectedResult bool
}

func testLogicalOperation(t *testing.T, f instruction, test OperandsLogical, s *state) {
	t.Helper()

	for _, operand := range test.operands {
		s.push(operand)
	}

	f(s)

	if test.expectedResult {
		assert.Equal(t, one, s.pop())
	} else {
		assert.Equal(t, zero, s.pop())
	}
}

type OperandsArithmetic struct {
	operands       []*big.Int
	expectedResult *big.Int
}

func testArithmeticOperation(t *testing.T, f instruction, test OperandsArithmetic, s *state) {
	t.Helper()

	for _, operand := range test.operands {
		s.push(operand)
	}

	f(s)

	assert.Equal(t, test.expectedResult, s.pop())
}

func TestAdd(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{one, one}, two},
		{[]*big.Int{zero, one}, one},
		{[]*big.Int{three, two}, five},
		{[]*big.Int{zero, zero}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opAdd, testOperand, s)
	}
}

func TestMul(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{two, two}, four},
		{[]*big.Int{three, two}, big.NewInt(6)},
		{[]*big.Int{three, one}, three},
		{[]*big.Int{zero, one}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opMul, testOperand, s)
	}
}

func TestSub(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{one, two}, one},
		{[]*big.Int{zero, two}, two},
		{[]*big.Int{two, two}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opSub, testOperand, s)
	}
}

func TestDiv(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{two, two}, one},
		{[]*big.Int{one, two}, two},
		{[]*big.Int{one, zero}, zero},
		{[]*big.Int{zero, one}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opDiv, testOperand, s)
	}
}

func TestSDiv(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{two, two}, one},
		{[]*big.Int{one, two}, two},
		{[]*big.Int{zero, one}, zero},
		{[]*big.Int{one, zero}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opSDiv, testOperand, s)
	}
}

func TestMod(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{two, three}, one},
		{[]*big.Int{two, two}, zero},
		{[]*big.Int{one, three}, zero},
		{[]*big.Int{zero, one}, zero},
		{[]*big.Int{three, five}, two},
	}
	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opMod, testOperand, s)
	}
}

func TestSMod(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{two, three}, one},
		{[]*big.Int{two, two}, zero},
		{[]*big.Int{one, three}, zero},
		{[]*big.Int{zero, one}, zero},
		{[]*big.Int{three, five}, two},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opSMod, testOperand, s)
	}
}

func TestExp(t *testing.T) {
	t.Run("EIP158", func(t *testing.T) {
		gasConsumed := 50
		startGas := 1000

		s, cancelFn := getState(&chain.ForksInTime{EIP158: true})
		defer cancelFn()

		testOperands := []OperandsArithmetic{
			{[]*big.Int{one, one}, one},
			{[]*big.Int{two, two}, four},
			{[]*big.Int{two, three}, big.NewInt(9)},
			{[]*big.Int{four, two}, big.NewInt(16)},
		}

		for i, testOperand := range testOperands {
			testArithmeticOperation(t, opExp, testOperand, s)
			assert.Equal(t, uint64(startGas-gasConsumed*(i+1)), s.gas)
		}
	})

	t.Run("NoForks", func(t *testing.T) {
		gasConsumed := 10
		startGas := 1000

		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		testOperands := []OperandsArithmetic{
			{[]*big.Int{one, one}, one},
			{[]*big.Int{two, two}, four},
			{[]*big.Int{two, three}, big.NewInt(9)},
			{[]*big.Int{four, two}, big.NewInt(16)},
		}

		for i, testOperand := range testOperands {
			testArithmeticOperation(t, opExp, testOperand, s)
			assert.Equal(t, uint64(startGas-gasConsumed*(i+1)), s.gas)
		}
	})
}

func TestAddMod(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{three, one, two}, zero},
		{[]*big.Int{two, one, two}, one},
		{[]*big.Int{zero, one, one}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opAddMod, testOperand, s)
	}
}

func TestMulMod(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{three, two, four}, two},
		{[]*big.Int{two, two, four}, zero},
		{[]*big.Int{zero, one, one}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opMulMod, testOperand, s)
	}
}

func TestAnd(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, true},
		{[]*big.Int{one, zero}, false},
		{[]*big.Int{zero, one}, false},
		{[]*big.Int{zero, zero}, false},
	}
	for _, testOperand := range testOperands {
		testLogicalOperation(t, opAnd, testOperand, s)
	}
}

func TestOr(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, true},
		{[]*big.Int{one, zero}, true},
		{[]*big.Int{zero, one}, true},
		{[]*big.Int{zero, zero}, false},
	}
	for _, testOperand := range testOperands {
		testLogicalOperation(t, opOr, testOperand, s)
	}
}

func TestXor(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{one, zero}, true},
		{[]*big.Int{zero, one}, true},
		{[]*big.Int{zero, zero}, false},
	}
	for _, testOperand := range testOperands {
		testLogicalOperation(t, opXor, testOperand, s)
	}
}

func TestByte(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{one, big.NewInt(31)}, one},
		{[]*big.Int{five, big.NewInt(31)}, five},
		{[]*big.Int{two, big.NewInt(32)}, zero},
		{[]*big.Int{one, big.NewInt(30)}, zero},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opByte, testOperand, s)
	}
}

func TestShl(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{Constantinople: true})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{three, one}, big.NewInt(6)},
		{[]*big.Int{three, zero}, three},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opShl, testOperand, s)
	}
}

func TestShr(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{Constantinople: true})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{five, one}, two},
		{[]*big.Int{five, two}, one},
		{[]*big.Int{five, zero}, five},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opShr, testOperand, s)
	}
}

func TestSar(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{Constantinople: true})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{five, one}, two},
		{[]*big.Int{five, two}, one},
		{[]*big.Int{five, zero}, five},
	}

	for _, testOperand := range testOperands {
		testArithmeticOperation(t, opSar, testOperand, s)
	}
}

func TestPush0(t *testing.T) {
	t.Run("single push0 success", func(t *testing.T) {
		s, closeFn := getState(&allEnabledForks)
		defer closeFn()

		opPush0(s)
		require.Equal(t, zero, s.pop())
	})

	t.Run("single push0 (EIP-3855 disabled)", func(t *testing.T) {
		allExceptEIP3855Fork := chain.AllForksEnabled.Copy().RemoveFork(chain.EIP3855).At(0)
		s, closeFn := getState(&allExceptEIP3855Fork)
		defer closeFn()

		opPush0(s)
		require.Error(t, errOpCodeNotFound, s.err)
	})

	t.Run("within stack size push0", func(t *testing.T) {
		s, closeFn := getState(&allEnabledForks)
		defer closeFn()

		for i := 0; i < stackSize; i++ {
			opPush0(s)
			require.NoError(t, s.err)
		}

		for i := 0; i < stackSize; i++ {
			require.Equal(t, zero, s.pop())
		}
	})
}

func TestGt(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{two, one}, false},
		{[]*big.Int{one, two}, true},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opGt, testOperand, s)
	}
}

func TestLt(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{two, one}, true},
		{[]*big.Int{one, two}, false},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opLt, testOperand, s)
	}
}

func TestEq(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{zero, zero}, true},
		{[]*big.Int{one, zero}, false},
		{[]*big.Int{zero, one}, false},
		{[]*big.Int{one, one}, true},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opEq, testOperand, s)
	}
}

func TestSlt(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{zero, one}, false},
		{[]*big.Int{one, zero}, true},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opSlt, testOperand, s)
	}
}

func TestSgt(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{zero, one}, true},
		{[]*big.Int{one, zero}, false},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opSgt, testOperand, s)
	}
}

func TestSignExtension(t *testing.T) {
	t.Run("BitAboveZero", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		firstValue, ok := new(big.Int).SetString("115792089237316195423570985008687907853269984665640564039457584007913129639808", 10)
		require.True(t, ok)
		secondValue, ok := new(big.Int).SetString("115792089237316195423570985008687907853269984665640564039457584007913129607168", 10)
		require.True(t, ok)
		thirdValue, ok := new(big.Int).SetString("115792089237316195423570985008687907853269984665640564039457584007913121251328", 10)
		require.True(t, ok)

		testOperands := []OperandsArithmetic{
			{[]*big.Int{big.NewInt(128), zero}, firstValue},
			{[]*big.Int{big.NewInt(32768), one}, secondValue},
			{[]*big.Int{big.NewInt(8388608), two}, thirdValue},
		}

		for _, testOperand := range testOperands {
			testArithmeticOperation(t, opSignExtension, testOperand, s)
		}
	})
	t.Run("BitZero", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		testOperands := []OperandsArithmetic{
			{[]*big.Int{one, two}, one},
			{[]*big.Int{two, one}, two},
			{[]*big.Int{two, zero}, two},
		}

		for _, testOperand := range testOperands {
			testArithmeticOperation(t, opSignExtension, testOperand, s)
		}
	})
}

func TestNot(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsArithmetic{
		{[]*big.Int{big.NewInt(-1)}, zero},
		{[]*big.Int{zero}, tt256m1},
		{[]*big.Int{one}, new(big.Int).Sub(tt256m1, big.NewInt(1))},
		{[]*big.Int{big.NewInt(10)}, new(big.Int).Sub(tt256m1, big.NewInt(10))},
	}
	for _, testOperand := range testOperands {
		t.Log(testOperand.expectedResult)
		testArithmeticOperation(t, opNot, testOperand, s)
	}
}

func TestIsZero(t *testing.T) {
	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	testOperands := []OperandsLogical{
		{[]*big.Int{one, one}, false},
		{[]*big.Int{zero, zero}, true},
		{[]*big.Int{two, two}, false},
	}

	for _, testOperand := range testOperands {
		testLogicalOperation(t, opIsZero, testOperand, s)
	}
}

func TestMStore(t *testing.T) {
	offset := big.NewInt(62)

	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	s.push(one)    // value
	s.push(offset) // offset

	opMStore(s)

	s.push(offset)

	opMLoad(s)

	assert.Equal(t, one, s.pop())
}

func TestMStore8(t *testing.T) {
	offsetStore := big.NewInt(62)
	offsetLoad := big.NewInt(31)

	s, closeFn := getState(&chain.ForksInTime{})
	defer closeFn()

	s.push(one)         //value
	s.push(offsetStore) //offset

	opMStore8(s)

	s.push(offsetLoad)

	opMLoad(s)

	assert.Equal(t, one, s.pop())
}

func TestSload(t *testing.T) {
	t.Run("Istanbul", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{Istanbul: true})
		defer closeFn()

		mockHost := &mockHost{}
		mockHost.On("GetStorage", mock.Anything, mock.Anything).Return(bigToHash(one)).Once()
		s.host = mockHost

		s.push(one)

		opSload(s)
		assert.Equal(t, uint64(200), s.gas)
		assert.Equal(t, bigToHash(one), bigToHash(s.pop()))
	})

	t.Run("EIP150", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{EIP150: true})
		defer closeFn()

		mockHost := &mockHost{}
		mockHost.On("GetStorage", mock.Anything, mock.Anything).Return(bigToHash(one)).Once()
		s.host = mockHost

		s.push(one)

		opSload(s)
		assert.Equal(t, uint64(800), s.gas)
		assert.Equal(t, bigToHash(one), bigToHash(s.pop()))
	})

	t.Run("NoForks", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{})
		defer closeFn()

		mockHost := &mockHost{}
		mockHost.On("GetStorage", mock.Anything, mock.Anything).Return(bigToHash(one)).Once()
		s.host = mockHost

		s.push(one)

		opSload(s)
		assert.Equal(t, uint64(950), s.gas)
		assert.Equal(t, bigToHash(one), bigToHash(s.pop()))
	})
}

func TestSStore(t *testing.T) {
	t.Run("ErrOutOfGas", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{
			Istanbul: true,
		})
		defer closeFn()

		s.push(one)

		opSStore(s)
		assert.True(t, s.stop)
		assert.Equal(t, errOutOfGas, s.err)
	})
	t.Run("StorageUnchanged", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{
			Istanbul:       true,
			Constantinople: true,
		})
		defer closeFn()

		s.gas = 10000

		mockHost := &mockHost{}
		mockHost.On("SetStorage", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything).Return(runtime.StorageUnchanged).Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)

		opSStore(s)
		assert.Equal(t, uint64(9200), s.gas)
	})
	t.Run("StorageModified", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{
			Istanbul:       true,
			Constantinople: true,
		})
		defer closeFn()

		s.gas = 10000

		mockHost := &mockHost{}
		mockHost.On("SetStorage", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything).Return(runtime.StorageModified).Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)

		opSStore(s)
		assert.Equal(t, uint64(5000), s.gas)
	})
	t.Run("StorageAdded", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{Istanbul: true, Constantinople: true})
		defer closeFn()

		s.gas = 25000

		mockHost := &mockHost{}
		mockHost.On("SetStorage", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything).Return(runtime.StorageAdded).Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)

		opSStore(s)
		assert.Equal(t, uint64(5000), s.gas)
	})
	t.Run("StorageDeleted", func(t *testing.T) {
		s, closeFn := getState(&chain.ForksInTime{
			Istanbul:       true,
			Constantinople: true,
		})
		defer closeFn()

		s.gas = 10000

		mockHost := &mockHost{}
		mockHost.On("SetStorage", mock.Anything, mock.Anything,
			mock.Anything, mock.Anything).Return(runtime.StorageDeleted).Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)

		opSStore(s)
		assert.Equal(t, uint64(5000), s.gas)
	})
}

func TestBalance(t *testing.T) {
	balance := big.NewInt(100)

	createMockHost := func() *mockHost {
		mockHost := &mockHost{}
		mockHost.On("GetBalance", mock.Anything).Return(balance)

		return mockHost
	}

	t.Run("Istanbul", func(t *testing.T) {
		gasLeft := uint64(300)

		s, cancelFn := getState(&chain.ForksInTime{Istanbul: true})
		defer cancelFn()

		s.host = createMockHost()

		opBalance(s)

		assert.Equal(t, balance, s.pop())
		assert.Equal(t, gasLeft, s.gas)
	})

	t.Run("EIP150", func(t *testing.T) {
		gasLeft := uint64(600)

		s, cancelFn := getState(&chain.ForksInTime{EIP150: true})
		defer cancelFn()

		s.host = createMockHost()

		opBalance(s)

		assert.Equal(t, big.NewInt(100), s.pop())
		assert.Equal(t, gasLeft, s.gas)
	})

	t.Run("OtherForks", func(t *testing.T) {
		gasLeft := uint64(980)

		s, cancelFn := getState(&chain.ForksInTime{London: true})
		defer cancelFn()

		s.host = createMockHost()

		opBalance(s)

		assert.Equal(t, balance, s.pop())
		assert.Equal(t, gasLeft, s.gas)
	})
}

func TestSelfBalance(t *testing.T) {
	balance := big.NewInt(100)

	t.Run("IstanbulFork", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{Istanbul: true})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetBalance", mock.Anything).Return(balance).Once()
		s.host = mockHost

		opSelfBalance(s)

		assert.Equal(t, big.NewInt(100), s.pop())
	})

	t.Run("NoForkErrorExpected", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetBalance", mock.Anything).Return(balance).Once()
		s.host = mockHost

		opSelfBalance(s)

		assert.True(t, s.stop)
		assert.Equal(t, s.err, errOpCodeNotFound)
	})
}

func TestChainID(t *testing.T) {
	chainID := int64(4)

	t.Run("IstanbulFork", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{Istanbul: true})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetTxContext").Return(runtime.TxContext{ChainID: 4}).Once()
		s.host = mockHost

		opChainID(s)

		assert.Equal(t, big.NewInt(chainID), s.pop())
	})
	t.Run("NoForksErrorExpected", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetTxContext").Return(runtime.TxContext{ChainID: 4}).Once()
		s.host = mockHost

		opChainID(s)

		assert.True(t, s.stop)
		assert.Equal(t, s.err, errOpCodeNotFound)
	})
}

func TestOrigin(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Origin: types.StringToAddress("0x1")}).Once()
	s.host = mockHost

	opOrigin(s)

	addr, ok := s.popAddr()
	assert.True(t, ok)
	assert.Equal(t, types.StringToAddress("0x1").Bytes(), addr.Bytes())
}

func TestCaller(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	callerAddr := types.StringToAddress("0xabcd")
	s.msg.Caller = callerAddr

	opCaller(s)

	addr, ok := s.popAddr()
	assert.True(t, ok)
	assert.Equal(t, callerAddr, addr)
}

func TestCallValue(t *testing.T) {
	t.Run("Msg Value non nil", func(t *testing.T) {
		value := big.NewInt(10)

		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.msg.Value = value

		opCallValue(s)
		assert.Equal(t, value, s.pop())
	})

	t.Run("Msg Value nil", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		opCallValue(s)
		assert.Equal(t, zero, s.pop())
	})
}

func TestCallDataLoad(t *testing.T) {
	t.Run("NonZeroOffset", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.push(one)

		s.msg = &runtime.Contract{Input: big.NewInt(7).Bytes()}

		opCallDataLoad(s)
		assert.Equal(t, zero, s.pop())
	})
	t.Run("ZeroOffset", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.push(zero)

		s.msg = &runtime.Contract{Input: big.NewInt(7).Bytes()}

		opCallDataLoad(s)
		assert.NotEqual(t, zero, s.pop())
	})
}

func TestCallDataSize(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.msg.Input = make([]byte, 10)

	opCallDataSize(s)
	assert.Equal(t, big.NewInt(10), s.pop())
}

func TestCodeSize(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.code = make([]byte, 10)

	opCodeSize(s)
	assert.Equal(t, big.NewInt(10), s.pop())
}

func TestExtCodeSize(t *testing.T) {
	codeSize := 10

	t.Run("EIP150", func(t *testing.T) {
		gasLeft := uint64(300)

		s, cancelFn := getState(&chain.ForksInTime{EIP150: true})
		defer cancelFn()
		s.push(one)

		mockHost := &mockHost{}
		mockHost.On("GetCodeSize", types.StringToAddress("0x1")).Return(codeSize).Once()
		s.host = mockHost

		opExtCodeSize(s)

		assert.Equal(t, gasLeft, s.gas)
		assert.Equal(t, big.NewInt(int64(codeSize)), s.pop())
	})
	t.Run("NoForks", func(t *testing.T) {
		gasLeft := uint64(980)

		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.push(one)

		mockHost := &mockHost{}
		mockHost.On("GetCodeSize", types.StringToAddress("0x1")).Return(codeSize).Once()
		s.host = mockHost

		opExtCodeSize(s)

		assert.Equal(t, gasLeft, s.gas)
		assert.Equal(t, big.NewInt(int64(codeSize)), s.pop())
	})
}

func TestGasPrice(t *testing.T) {
	gasPrice := int64(10)

	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{GasPrice: bigToHash(big.NewInt(gasPrice))}).Once()
	s.host = mockHost

	opGasPrice(s)

	assert.Equal(t, bigToHash(big.NewInt(gasPrice)), s.popHash())
}

func TestReturnDataSize(t *testing.T) {
	dataSize := int64(1024)

	t.Run("Byzantium", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{Byzantium: true})
		defer cancelFn()

		s.returnData = make([]byte, dataSize)

		opReturnDataSize(s)

		assert.Equal(t, big.NewInt(dataSize), s.pop())
	})
	t.Run("NoForks", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.returnData = make([]byte, dataSize)

		opReturnDataSize(s)

		assert.True(t, s.stop)
		assert.Equal(t, errOpCodeNotFound, s.err)
	})
}

func TestExtCodeHash(t *testing.T) {
	t.Run("Istanbul", func(t *testing.T) {
		gasLeft := uint64(300)

		s, cancelFn := getState(&chain.ForksInTime{
			Constantinople: true,
			Istanbul:       true,
		})
		defer cancelFn()

		s.push(one)

		mockHost := &mockHost{}
		mockHost.On("Empty", types.StringToAddress("0x1")).Return(false).Once()
		mockHost.On("GetCodeHash", types.StringToAddress("0x1")).Return("0x1").Once()
		s.host = mockHost

		opExtCodeHash(s)

		assert.Equal(t, s.gas, gasLeft)
		assert.Equal(t, one, s.pop())
	})

	t.Run("NonIstanbul", func(t *testing.T) {
		gasLeft := uint64(600)

		s, cancelFn := getState(&chain.ForksInTime{
			Constantinople: true,
		})
		defer cancelFn()

		s.push(one)

		mockHost := &mockHost{}
		mockHost.On("Empty", mock.Anything).Return(true).Once()
		s.host = mockHost

		opExtCodeHash(s)
		assert.Equal(t, gasLeft, s.gas)
		assert.Equal(t, zero, s.pop())
	})

	t.Run("NoForks", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.push(one)

		mockHost := &mockHost{}
		mockHost.On("Empty", mock.Anything).Return(true).Once()
		s.host = mockHost

		opExtCodeHash(s)
		assert.True(t, s.stop)
		assert.Equal(t, errOpCodeNotFound, s.err)
	})
}

func TestPCMSizeGas(t *testing.T) {
	memorySize := uint64(1024)
	gasLeft := uint64(1000)

	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	t.Run("PC", func(t *testing.T) {
		s.ip = 1
		opPC(s)

		assert.Equal(t, one, s.pop())
	})

	t.Run("MSize", func(t *testing.T) {
		s.memory = make([]byte, memorySize)

		opMSize(s)

		assert.Equal(t, new(big.Int).SetUint64(memorySize), s.pop())
	})

	t.Run("Gas", func(t *testing.T) {
		opGas(s)

		assert.Equal(t, new(big.Int).SetUint64(gasLeft), s.pop())
	})
}

func TestExtCodeCopy(t *testing.T) {
	t.Run("EIP150", func(t *testing.T) {
		leftGas := uint64(294)

		s, cancelFn := getState(&chain.ForksInTime{EIP150: true})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetCode", mock.Anything).Return("0x1").Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)
		s.push(big.NewInt(31))
		s.push(big.NewInt(32))

		opExtCodeCopy(s)

		assert.Equal(t, leftGas, s.gas)
		assert.Equal(t, big.NewInt(1).FillBytes(make([]byte, 32)), s.memory)
	})

	t.Run("NonEIP150Fork", func(t *testing.T) {
		leftGas := uint64(974)
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetCode", mock.Anything).Return("0x1").Once()
		s.host = mockHost

		s.push(one)
		s.push(zero)
		s.push(big.NewInt(31))
		s.push(big.NewInt(32))

		opExtCodeCopy(s)

		assert.Equal(t, leftGas, s.gas)
		assert.Equal(t, big.NewInt(1).FillBytes(make([]byte, 32)), s.memory)
	})
}

func TestCallDataCopy(t *testing.T) {
	gasLeft := uint64(994)

	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.msg.Input = one.Bytes()

	s.push(big.NewInt(1))
	s.push(zero)
	s.push(big.NewInt(31))

	opCallDataCopy(s)

	assert.Equal(t, gasLeft, s.gas)
	assert.Equal(t, big.NewInt(1).FillBytes(make([]byte, 32)), s.memory)
}

func TestCodeCopyLenZero(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	var expectedGas = s.gas

	s.push(big.NewInt(0)) //length
	s.push(big.NewInt(0)) //dataOffset
	s.push(big.NewInt(0)) //memOffset

	opCodeCopy(s)

	// We check that no gas was spent and there was no error
	assert.Equal(t, expectedGas, s.gas)
	assert.NoError(t, s.err)
}

func TestCodeCopy(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.push(big.NewInt(1))  //length
	s.push(zero)           //dataOffset
	s.push(big.NewInt(31)) //memOffset

	s.code = one.Bytes()

	opCodeCopy(s)
	assert.Equal(t, one.FillBytes(make([]byte, 32)), s.memory)
}

func TestBlockHash(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.push(three)

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Number: 5}).Once()
	mockHost.On("GetBlockHash", mock.Anything).Return(bigToHash(three)).Once()
	s.host = mockHost

	opBlockHash(s)

	assert.Equal(t, bigToHash(three), bigToHash(s.pop()))
}

func TestCoinBase(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Coinbase: types.StringToAddress("0x1")}).Once()
	s.host = mockHost

	opCoinbase(s)

	assert.Equal(t, types.StringToAddress("0x1").Bytes(), s.pop().FillBytes(make([]byte, 20)))
}

func TestTimeStamp(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Timestamp: 335}).Once()
	s.host = mockHost

	opTimestamp(s)

	assert.Equal(t, big.NewInt(335), s.pop())
}

func TestNumber(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Number: 5}).Once()
	s.host = mockHost

	opNumber(s)

	assert.Equal(t, five, s.pop())
}

func TestDifficulty(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	mockHost := &mockHost{}
	mockHost.On("GetTxContext").Return(runtime.TxContext{Difficulty: bigToHash(five)}).Once()
	s.host = mockHost

	opDifficulty(s)

	assert.Equal(t, bigToHash(five), bigToHash(s.pop()))
}

func TestGasLimit(t *testing.T) {
	baseFee := uint64(11)

	t.Run("NonLondonFork", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetTxContext").Return(runtime.TxContext{GasLimit: 11}).Once()
		s.host = mockHost

		opBaseFee(s)
		assert.EqualError(t, errOpCodeNotFound, s.err.Error())
	})

	t.Run("LondonFork", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{London: true})
		defer cancelFn()

		mockHost := &mockHost{}
		mockHost.On("GetTxContext").Return(runtime.TxContext{BaseFee: big.NewInt(11)}).Once()
		s.host = mockHost

		opBaseFee(s)

		assert.Equal(t, new(big.Int).SetUint64(baseFee), s.pop())
	})
}

func TestSelfDestruct(t *testing.T) {
	addr := types.StringToAddress("0x1")

	s, cancelFn := getState(&chain.ForksInTime{
		EIP150: true,
		EIP158: true})
	defer cancelFn()

	s.msg.Address = types.StringToAddress("0x2")

	s.gas = 100000
	s.push(one)

	mockHost := &mockHost{}
	mockHost.On("Empty", addr).Return(true).Once()
	mockHost.On("Selfdestruct", mock.Anything, mock.Anything)
	mockHost.On("GetBalance", types.StringToAddress("0x2")).Return(big.NewInt(100)).Once()
	s.host = mockHost

	opSelfDestruct(s)

	assert.Equal(t, uint64(70000), s.gas)
	assert.True(t, s.stop)
}

func TestJump(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.code = make([]byte, 10)
	s.bitmap = bitmap{big.NewInt(255).Bytes()}
	s.push(five)

	opJump(s)

	assert.Equal(t, 4, s.ip)
}

func TestJumpI(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.code = make([]byte, 10)
	s.bitmap = bitmap{big.NewInt(255).Bytes()}
	s.push(one)
	s.push(five)

	opJumpi(s)

	assert.Equal(t, 4, s.ip)
}

func TestDup(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.sp = 6

	for i := 0; i < 10; i++ {
		s.stack = append(s.stack, big.NewInt(int64(i)))
	}

	instr := opDup(4)
	instr(s)

	assert.Equal(t, two, s.pop())
}

func TestSwap(t *testing.T) {
	s, cancelFn := getState(&chain.ForksInTime{})
	defer cancelFn()

	s.sp = 6

	for i := 0; i < 10; i++ {
		s.stack = append(s.stack, big.NewInt(int64(i)))
	}

	instr := opSwap(4)
	instr(s)

	assert.Equal(t, five, s.stack[1])
	assert.Equal(t, one, s.stack[6-1])
}

func TestLog(t *testing.T) {
	t.Run("StaticCall", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.msg.Static = true
		s.sp = 1

		s.push(big.NewInt(3))
		s.push(big.NewInt(20))

		for i := 0; i < 20; i++ {
			s.push(big.NewInt(int64(i)))
		}

		instr := opLog(10)
		instr(s)

		assert.Equal(t, errWriteProtection, s.err)
	})

	t.Run("StackUnderflow", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.sp = 1

		s.push(big.NewInt(3))
		s.push(big.NewInt(20))

		for i := 0; i < 20; i++ {
			s.push(big.NewInt(int64(i)))
		}

		instr := opLog(35)
		instr(s)

		assert.Error(t, s.err)
	})

	t.Run("Log", func(t *testing.T) {
		s, cancelFn := getState(&chain.ForksInTime{})
		defer cancelFn()

		s.gas = 25000

		s.push(big.NewInt(3))
		s.push(big.NewInt(20))

		mockHost := &mockHost{}
		mockHost.On("EmitLog", mock.Anything, mock.Anything, mock.Anything).Once()
		s.host = mockHost

		for i := 0; i < 20; i++ {
			s.push(big.NewInt(int64(i)))
		}

		instr := opLog(10)
		instr(s)

		assert.Equal(t, uint64(21475), s.gas)
	})
}

type mockHostForInstructions struct {
	mockHost
	nonce       uint64
	code        []byte
	callxResult *runtime.ExecutionResult
	addresses   map[types.Address]int
	storages    []map[types.Hash]types.Hash
}

func (m *mockHostForInstructions) GetNonce(types.Address) uint64 {
	return m.nonce
}

func (m *mockHostForInstructions) Callx(*runtime.Contract, runtime.Host) *runtime.ExecutionResult {
	return m.callxResult
}

func (m *mockHostForInstructions) GetCode(addr types.Address) []byte {
	return m.code
}

func (m *mockHostForInstructions) GetStorage(addr types.Address, key types.Hash) types.Hash {
	idx, ok := m.addresses[addr]
	if !ok {
		return types.ZeroHash
	}

	res, ok := m.storages[idx][key]
	if !ok {
		return types.ZeroHash
	}

	return res
}

var (
	addr1 = types.StringToAddress("1")
)

func Test_opSload(t *testing.T) {
	t.Parallel()

	type state struct {
		gas        uint64
		sp         int
		stack      []*big.Int
		memory     []byte
		accessList *runtime.AccessList
		stop       bool
		err        error
	}

	address1 := types.StringToAddress("address1")
	key1 := types.StringToHash("1")
	val1 := types.StringToHash("2")
	tests := []struct {
		name        string
		op          OpCode
		contract    *runtime.Contract
		config      *chain.ForksInTime
		initState   *state
		resultState *state
		mockHost    *mockHostForInstructions
	}{
		{
			name: "charge ColdStorageReadCostEIP2929 if the (address, storage_key) pair is not accessed_storage_keys",
			op:   SLOAD,
			contract: &runtime.Contract{
				Address: address1,
				Journal: &runtime.Journal{},
			},
			config: &chain.ForksInTime{
				Berlin: true,
			},
			initState: &state{
				gas: 10000,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(key1.Bytes()),
				},
				memory:     []byte{0x01},
				accessList: runtime.NewAccessList(),
			},
			resultState: &state{
				gas: 7900,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(val1.Bytes()),
				},
				memory: []byte{0x01},
				stop:   false,
				err:    nil,
				accessList: &runtime.AccessList{
					address1: {
						key1: struct{}{},
					},
				},
			},
			mockHost: &mockHostForInstructions{
				addresses: map[types.Address]int{
					address1: 0,
				},
				storages: []map[types.Hash]types.Hash{
					{
						key1: val1,
					},
				},
			},
		},
		{
			name: "charge WarmStorageReadCostEIP2929 if the (address, storage_key) pair is in access list",
			op:   SLOAD,
			contract: &runtime.Contract{
				Address: address1,
				Journal: &runtime.Journal{},
			},
			config: &chain.ForksInTime{
				Berlin: true,
			},
			initState: &state{
				gas: 10000,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(key1.Bytes()),
				},
				memory: []byte{0x01},
				accessList: &runtime.AccessList{
					address1: {
						key1: struct{}{},
					},
				},
			},
			resultState: &state{
				gas: 9900,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(val1.Bytes()),
				},
				memory: []byte{0x01},
				stop:   false,
				err:    nil,
				accessList: &runtime.AccessList{
					address1: {
						key1: struct{}{},
					},
				},
			},
			mockHost: &mockHostForInstructions{
				addresses: map[types.Address]int{
					address1: 0,
				},
				storages: []map[types.Hash]types.Hash{
					{
						key1: val1,
					},
				},
			},
		},
		{
			name: "charge Gas 800 when EIP2929 is not enabled and Istanbul is enabled",
			op:   SLOAD,
			contract: &runtime.Contract{
				Address: address1,
				Journal: &runtime.Journal{},
			},
			config: &chain.ForksInTime{
				Berlin:   false,
				Istanbul: true,
			},
			initState: &state{
				gas: 10000,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(key1.Bytes()),
				},
				memory:     []byte{0x01},
				accessList: nil,
			},
			resultState: &state{
				gas: 9200,
				sp:  1,
				stack: []*big.Int{
					new(big.Int).SetBytes(val1.Bytes()),
				},
				memory:     []byte{0x01},
				stop:       false,
				err:        nil,
				accessList: nil,
			},
			mockHost: &mockHostForInstructions{
				addresses: map[types.Address]int{
					address1: 0,
				},
				storages: []map[types.Hash]types.Hash{
					{
						key1: val1,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, closeFn := getState(tt.config)
			defer closeFn()
			s.msg = tt.contract
			s.gas = tt.initState.gas
			s.sp = tt.initState.sp
			s.stack = tt.initState.stack
			s.memory = tt.initState.memory
			s.config = tt.config
			s.host = tt.mockHost
			tt.contract.AccessList = tt.initState.accessList
			opSload(s)
			assert.Equal(t, tt.resultState.gas, s.gas, "gas in state after execution is not correct")
			assert.Equal(t, tt.resultState.sp, s.sp, "sp in state after execution is not correct")
			assert.Equal(t, tt.resultState.stack, s.stack, "stack in state after execution is not correct")
			assert.Equal(t, tt.resultState.memory, s.memory, "memory in state after execution is not correct")
			assert.Equal(t, tt.resultState.accessList, tt.contract.AccessList, "accesslist in state after execution is not correct")
			assert.Equal(t, tt.resultState.stop, s.stop, "stop in state after execution is not correct")
			assert.Equal(t, tt.resultState.err, s.err, "err in state after execution is not correct")
		})
	}
}

func TestCreate(t *testing.T) {
	type state struct {
		gas    uint64
		sp     int
		stack  []*big.Int
		memory []byte
		stop   bool
		err    error
	}

	addressToBigInt := func(addr types.Address) *big.Int {
		return new(big.Int).SetBytes(addr[:])
	}

	tests := []struct {
		name        string
		op          OpCode
		contract    *runtime.Contract
		config      *chain.ForksInTime
		initState   *state
		resultState *state
		mockHost    *mockHostForInstructions
	}{
		{
			name: "should succeed in case of CREATE",
			op:   CREATE,
			contract: &runtime.Contract{
				Static:  false,
				Address: addr1,
			},
			config: &chain.ForksInTime{},
			initState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
			},
			resultState: &state{
				gas: 500,
				sp:  1,
				stack: []*big.Int{
					addressToBigInt(crypto.CreateAddress(addr1, 0)), // contract address
					big.NewInt(0x00),
					big.NewInt(0x00),
				},
				memory: []byte{
					byte(REVERT),
				},
			},
			mockHost: &mockHostForInstructions{
				nonce: 0,
				callxResult: &runtime.ExecutionResult{
					GasLeft: 500,
					GasUsed: 500,
				},
			},
		},
		{
			name: "should throw errWriteProtection in case of static call",
			op:   CREATE,
			contract: &runtime.Contract{
				Static: true,
			},
			config: &chain.ForksInTime{},
			initState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			// shouldn't change any states except for stop and err
			resultState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: true,
				err:  errWriteProtection,
			},
			mockHost: &mockHostForInstructions{},
		},
		{
			name:     "should throw errOpCodeNotFound when op is CREATE2 and config.Constantinople is disabled",
			op:       CREATE2,
			contract: &runtime.Contract{},
			config: &chain.ForksInTime{
				Constantinople: false,
			},
			initState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			// shouldn't change any states except for stop and err
			resultState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: true,
				err:  errOpCodeNotFound,
			},
			mockHost: &mockHostForInstructions{},
		},
		{
			name: "should set zero address if op is CREATE and contract call throws ErrCodeStoreOutOfGas",
			op:   CREATE,
			contract: &runtime.Contract{
				Static:  false,
				Address: addr1,
			},
			config: &chain.ForksInTime{
				Homestead: true,
			},
			initState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			// shouldn't change any states except for stop and err
			resultState: &state{
				gas: 1000,
				sp:  1,
				stack: []*big.Int{
					// need to init with 0x01 to add abs field in big.Int
					big.NewInt(0x01).SetInt64(0x00),
					big.NewInt(0x00),
					big.NewInt(0x00),
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			mockHost: &mockHostForInstructions{
				nonce: 0,
				callxResult: &runtime.ExecutionResult{
					GasLeft: 1000,
					Err:     runtime.ErrCodeStoreOutOfGas,
				},
			},
		},
		{
			name: "should set zero address if contract call throws error except for ErrCodeStoreOutOfGas",
			op:   CREATE,
			contract: &runtime.Contract{
				Static:  false,
				Address: addr1,
			},
			config: &chain.ForksInTime{
				Homestead: true,
			},
			initState: &state{
				gas: 1000,
				sp:  3,
				stack: []*big.Int{
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			// shouldn't change any states except for stop and err
			resultState: &state{
				gas: 1000,
				sp:  1,
				stack: []*big.Int{
					// need to init with 0x01 to add abs field in big.Int
					big.NewInt(0x01).SetInt64(0x00),
					big.NewInt(0x00),
					big.NewInt(0x00),
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			mockHost: &mockHostForInstructions{
				nonce: 0,
				callxResult: &runtime.ExecutionResult{
					GasLeft: 1000,
					Err:     errRevert,
				},
			},
		},
		{
			name: "should set zero address if contract call throws any error for CREATE2",
			op:   CREATE2,
			contract: &runtime.Contract{
				Static:  false,
				Address: addr1,
			},
			config: &chain.ForksInTime{
				Homestead:      true,
				Constantinople: true,
			},
			initState: &state{
				gas: 1000,
				sp:  4,
				stack: []*big.Int{
					big.NewInt(0x01), // salt
					big.NewInt(0x01), // length
					big.NewInt(0x00), // offset
					big.NewInt(0x00), // value
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			// during creation of code with length 1 for CREATE2 op code, 985 gas units are spent by buildCreateContract()
			resultState: &state{
				gas: 15,
				sp:  1,
				stack: []*big.Int{
					big.NewInt(0x01).SetInt64(0x00),
					big.NewInt(0x01),
					big.NewInt(0x00),
					big.NewInt(0x00),
				},
				memory: []byte{
					byte(REVERT),
				},
				stop: false,
				err:  nil,
			},
			mockHost: &mockHostForInstructions{
				nonce: 0,
				callxResult: &runtime.ExecutionResult{
					// if it is ErrCodeStoreOutOfGas then we set GasLeft to 0
					GasLeft: 0,
					Err:     runtime.ErrCodeStoreOutOfGas,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, closeFn := getState(&chain.ForksInTime{})
			defer closeFn()

			s.msg = tt.contract
			s.gas = tt.initState.gas
			s.sp = tt.initState.sp
			s.stack = tt.initState.stack
			s.memory = tt.initState.memory
			s.config = tt.config
			s.host = tt.mockHost

			opCreate(tt.op)(s)

			assert.Equal(t, tt.resultState.gas, s.gas, "gas in state after execution is not correct")
			assert.Equal(t, tt.resultState.sp, s.sp, "sp in state after execution is not correct")
			assert.Equal(t, tt.resultState.stack, s.stack, "stack in state after execution is not correct")
			assert.Equal(t, tt.resultState.memory, s.memory, "memory in state after execution is not correct")
			assert.Equal(t, tt.resultState.stop, s.stop, "stop in state after execution is not correct")
			assert.Equal(t, tt.resultState.err, s.err, "err in state after execution is not correct")
		})
	}
}

func Test_opReturnDataCopy(t *testing.T) {
	t.Parallel()

	// Positive number that does not fit in uint64 (math.MaxUint64 + 1)
	largeNumber := "18446744073709551616"
	bigIntValue := new(big.Int)
	bigIntValue.SetString(largeNumber, 10)

	// Positive number that does fit in uint64 but multiplied by two does not
	largeNumber2 := "18446744073709551615"
	bigIntValue2 := new(big.Int)
	bigIntValue2.SetString(largeNumber2, 10)

	tests := []struct {
		name        string
		config      *chain.ForksInTime
		initState   *state
		resultState *state
	}{
		{
			name: "should return error if Byzantium is not applied",
			config: &chain.ForksInTime{
				Byzantium: false,
			},
			initState: &state{},
			resultState: &state{
				config: &chain.ForksInTime{
					Byzantium: false,
				},
				stop: true,
				err:  errOpCodeNotFound,
			},
		},
		{
			name:   "should return error if memOffset is negative",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(1),  // length
					big.NewInt(0),  // dataOffset
					big.NewInt(-1), // memOffset
				},
				sp:         3,
				returnData: []byte{0xff},
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(1),
					big.NewInt(0),
					big.NewInt(-1),
				},
				sp:         0,
				returnData: []byte{0xff},
				stop:       true,
				err:        errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should return error if dataOffset is negative",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(1),  // length
					big.NewInt(-1), // dataOffset
					big.NewInt(0),  // memOffset
				},
				sp:     3,
				memory: make([]byte, 1),
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(1),
					big.NewInt(-1),
					big.NewInt(0),
				},
				sp:     0,
				memory: make([]byte, 1),
				stop:   true,
				err:    errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should return error if length is negative",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(-1), // length
					big.NewInt(2),  // dataOffset
					big.NewInt(0),  // memOffset
				},
				sp:         3,
				returnData: []byte{0xff},
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(-1),
					big.NewInt(2),
					big.NewInt(0),
				},
				sp:         0,
				returnData: []byte{0xff},
				stop:       true,
				err:        errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should copy data from returnData to memory",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(1), // length
					big.NewInt(0), // dataOffset
					big.NewInt(0), // memOffset
				},
				sp:         3,
				returnData: []byte{0xff},
				memory:     []byte{0x0},
				gas:        10,
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(1),
					big.NewInt(0),
					big.NewInt(0),
				},
				sp:                 0,
				returnData:         []byte{0xff},
				memory:             []byte{0xff},
				gas:                7,
				lastGasCost:        0,
				currentConsumedGas: 3,
				stop:               false,
				err:                nil,
			},
		},
		{
			// this test case also verifies that code does not panic when the length is 0 and memOffset > len(memory)
			name:   "should not copy data if length is zero",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(0), // length
					big.NewInt(0), // dataOffset
					big.NewInt(4), // memOffset
				},
				sp:         3,
				returnData: []byte{0x01},
				memory:     []byte{0x02},
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(0),
					big.NewInt(0),
					big.NewInt(4),
				},
				sp:         0,
				returnData: []byte{0x01},
				memory:     []byte{0x02},
				stop:       false,
				err:        nil,
			},
		},
		{
			name:   "should return error if data offset overflows uint64",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(1),  // length
					bigIntValue,    // dataOffset
					big.NewInt(-1), // memOffset
				},
				sp: 3,
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(1),
					bigIntValue,
					big.NewInt(-1),
				},
				sp:   0,
				stop: true,
				err:  errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should return error if sum of data offset and length overflows uint64",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					bigIntValue2,   // length
					bigIntValue2,   // dataOffset
					big.NewInt(-1), // memOffset
				},
				sp: 3,
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					bigIntValue2,
					bigIntValue2,
					big.NewInt(-1),
				},
				sp:   0,
				stop: true,
				err:  errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should return error if the length of return data does not have enough space to receive offset + length bytes",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(2), // length
					big.NewInt(0), // dataOffset
					big.NewInt(0), // memOffset
				},
				sp:         3,
				returnData: []byte{0xff},
				memory:     []byte{0x0},
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(2),
					big.NewInt(0),
					big.NewInt(0),
				},
				sp:         0,
				returnData: []byte{0xff},
				memory:     []byte{0x0},
				stop:       true,
				err:        errReturnDataOutOfBounds,
			},
		},
		{
			name:   "should return error if there is no gas",
			config: &allEnabledForks,
			initState: &state{
				stack: []*big.Int{
					big.NewInt(1), // length
					big.NewInt(0), // dataOffset
					big.NewInt(0), // memOffset
				},
				sp:         3,
				returnData: []byte{0xff},
				memory:     []byte{0x0},
				gas:        0,
			},
			resultState: &state{
				config: &allEnabledForks,
				stack: []*big.Int{
					big.NewInt(1),
					big.NewInt(0),
					big.NewInt(0),
				},
				sp:                 0,
				returnData:         []byte{0xff},
				memory:             []byte{0x0},
				gas:                0,
				currentConsumedGas: 3,
				stop:               true,
				err:                errOutOfGas,
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			state, closeFn := getState(&chain.ForksInTime{})
			defer closeFn()

			state.gas = test.initState.gas
			state.sp = test.initState.sp
			state.stack = test.initState.stack
			state.memory = test.initState.memory
			state.returnData = test.initState.returnData
			state.config = test.config

			// assign nil to some fields in cached state object
			state.code = nil
			state.host = nil
			state.msg = nil
			state.tmp = nil
			state.bitmap = bitmap{}
			state.ret = nil
			state.currentConsumedGas = 0

			opReturnDataCopy(state)

			assert.Equal(t, test.resultState, state)
		})
	}
}

func Test_opCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		op          OpCode
		contract    *runtime.Contract
		config      chain.ForksInTime
		initState   *state
		resultState *state
		mockHost    *mockHostForInstructions
	}{
		{
			// this test case also verifies that code does not panic when the outSize is 0 and outOffset > len(memory)
			name: "should not copy result into memory if outSize is 0",
			op:   STATICCALL,
			contract: &runtime.Contract{
				Static:     true,
				Journal:    &runtime.Journal{},
				AccessList: runtime.NewAccessList(),
			},
			config: allEnabledForks,
			initState: &state{
				gas: 2600,
				sp:  6,
				stack: []*big.Int{
					big.NewInt(0x00), // outSize
					big.NewInt(0x02), // outOffset
					big.NewInt(0x00), // inSize
					big.NewInt(0x00), // inOffset
					big.NewInt(0x00), // address
					big.NewInt(0x00), // initialGas
				},
				memory: []byte{0x01},
			},
			resultState: &state{
				memory: []byte{0x01},
				stop:   false,
				err:    nil,
			},
			mockHost: &mockHostForInstructions{
				callxResult: &runtime.ExecutionResult{
					ReturnValue: []byte{0x03},
				},
			},
		},
		// {
		// 	name: "call cost overflow (EIP150 fork disabled)",
		// 	op:   CALLCODE,
		// 	contract: &runtime.Contract{
		// 		Static: false,
		// 	},
		// 	config: chain.AllForksEnabled.RemoveFork(chain.EIP150).At(0),
		// 	initState: &state{
		// 		gas: 6640,
		// 		sp:  7,
		// 		stack: []*big.Int{
		// 			big.NewInt(0x00),                        // outSize
		// 			big.NewInt(0x00),                        // outOffset
		// 			big.NewInt(0x00),                        // inSize
		// 			big.NewInt(0x00),                        // inOffset
		// 			big.NewInt(0x01),                        // value
		// 			big.NewInt(0x03),                        // address
		// 			big.NewInt(0).SetUint64(math.MaxUint64), // initialGas
		// 		},
		// 		memory:     []byte{0x01},
		// 		accessList: runtime.NewAccessList(),
		// 	},
		// 	resultState: &state{
		// 		memory: []byte{0x01},
		// 		stop:   true,
		// 		err:    errGasUintOverflow,
		// 		gas:    6640,
		// 	},
		// 	mockHost: &mockHostForInstructions{
		// 		callxResult: &runtime.ExecutionResult{
		// 			ReturnValue: []byte{0x03},
		// 		},
		// 	},
		// },
		// {
		// 	name: "available gas underflow",
		// 	op:   CALLCODE,
		// 	contract: &runtime.Contract{
		// 		Static: false,
		// 	},
		// 	config: allEnabledForks,
		// 	initState: &state{
		// 		gas: 6640,
		// 		sp:  7,
		// 		stack: []*big.Int{
		// 			big.NewInt(0x00),                        // outSize
		// 			big.NewInt(0x00),                        // outOffset
		// 			big.NewInt(0x00),                        // inSize
		// 			big.NewInt(0x00),                        // inOffset
		// 			big.NewInt(0x01),                        // value
		// 			big.NewInt(0x03),                        // address
		// 			big.NewInt(0).SetUint64(math.MaxUint64), // initialGas
		// 		},
		// 		memory:     []byte{0x01},
		// 		accessList: runtime.NewAccessList(),
		// 	},
		// 	resultState: &state{
		// 		memory: []byte{0x01},
		// 		stop:   true,
		// 		err:    errOutOfGas,
		// 		gas:    6640,
		// 	},
		// 	mockHost: &mockHostForInstructions{
		// 		callxResult: &runtime.ExecutionResult{
		// 			ReturnValue: []byte{0x03},
		// 		},
		// 	},
		// },
	}

	for _, tt := range tests {
		test := tt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			state, closeFn := getState(&tt.config)
			defer closeFn()

			state.gas = test.initState.gas
			state.msg = test.contract
			state.sp = test.initState.sp
			state.stack = test.initState.stack
			state.memory = test.initState.memory
			state.config = &test.config
			state.host = test.mockHost

			opCall(test.op)(state)

			assert.Equal(t, test.resultState.memory, state.memory, "memory in state after execution is incorrect")
			assert.Equal(t, test.resultState.stop, state.stop, "stop in state after execution is incorrect")
			assert.Equal(t, test.resultState.err, state.err, "err in state after execution is incorrect")
			assert.Equal(t, test.resultState.gas, state.gas, "gas in state after execution is incorrect")
		})
	}
}
