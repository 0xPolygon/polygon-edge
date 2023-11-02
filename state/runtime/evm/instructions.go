//nolint:forcetypeassert
package evm

import (
	"errors"
	"math/big"
	"math/bits"
	"sync"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

type instruction func(c *state)

var (
	zero     = big.NewInt(0)
	one      = big.NewInt(1)
	wordSize = big.NewInt(32)
)

func opAdd(c *state) {
	a := c.pop()
	b := c.top()

	b.Add(a, b)
	toU256(b)
}

func opMul(c *state) {
	a := c.pop()
	b := c.top()

	b.Mul(a, b)
	toU256(b)
}

func opSub(c *state) {
	a := c.pop()
	b := c.top()

	b.Sub(a, b)
	toU256(b)
}

func opDiv(c *state) {
	a := c.pop()
	b := c.top()

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		b.Div(a, b)
		toU256(b)
	}
}

func opSDiv(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		neg := a.Sign() != b.Sign()
		b.Div(a.Abs(a), b.Abs(b))
		if neg {
			b.Neg(b)
		}
		toU256(b)
	}
}

func opMod(c *state) {
	a := c.pop()
	b := c.top()

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		b.Mod(a, b)
		toU256(b)
	}
}

func opSMod(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if b.Sign() == 0 {
		// division by zero
		b.Set(zero)
	} else {
		neg := a.Sign() < 0
		b.Mod(a.Abs(a), b.Abs(b))
		if neg {
			b.Neg(b)
		}
		toU256(b)
	}
}

var bigPool = sync.Pool{
	New: func() interface{} {
		return new(big.Int)
	},
}

func acquireBig() *big.Int {
	return bigPool.Get().(*big.Int)
}

func releaseBig(b *big.Int) {
	bigPool.Put(b)
}

func opExp(c *state) {
	x := c.pop()
	y := c.top()

	var gas uint64
	if c.config.EIP158 {
		gas = 50
	} else {
		gas = 10
	}

	gasCost := uint64((y.BitLen()+7)/8) * gas
	if !c.consumeGas(gasCost) {
		return
	}

	z := acquireBig().Set(one)

	// https://www.programminglogic.com/fast-exponentiation-algorithms/
	for _, d := range y.Bits() {
		for i := 0; i < _W; i++ {
			if d&1 == 1 {
				toU256(z.Mul(z, x))
			}

			d >>= 1

			toU256(x.Mul(x, x))
		}
	}

	y.Set(z)
	releaseBig(z)
}

func opAddMod(c *state) {
	a := c.pop()
	b := c.pop()
	z := c.top()

	if z.Sign() == 0 {
		// division by zero
		z.Set(zero)
	} else {
		a = a.Add(a, b)
		z = z.Mod(a, z)
		toU256(z)
	}
}

func opMulMod(c *state) {
	a := c.pop()
	b := c.pop()
	z := c.top()

	if z.Sign() == 0 {
		// division by zero
		z.Set(zero)
	} else {
		a = a.Mul(a, b)
		z = z.Mod(a, z)
		toU256(z)
	}
}

func opAnd(c *state) {
	a := c.pop()
	b := c.top()

	b.And(a, b)
}

func opOr(c *state) {
	a := c.pop()
	b := c.top()

	b.Or(a, b)
}

func opXor(c *state) {
	a := c.pop()
	b := c.top()

	b.Xor(a, b)
}

var opByteMask = big.NewInt(255)

func opByte(c *state) {
	x := c.pop()
	y := c.top()

	indx := x.Int64()
	if indx > 31 {
		y.Set(zero)
	} else {
		sh := (31 - indx) * 8
		y.Rsh(y, uint(sh))
		y.And(y, opByteMask)
	}
}

func opNot(c *state) {
	a := c.top()

	a.Not(a)
	toU256(a)
}

func opIsZero(c *state) {
	a := c.top()

	if a.Sign() == 0 {
		a.Set(one)
	} else {
		a.Set(zero)
	}
}

func opEq(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) == 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opLt(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) < 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opGt(c *state) {
	a := c.pop()
	b := c.top()

	if a.Cmp(b) > 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSlt(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if a.Cmp(b) < 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSgt(c *state) {
	a := to256(c.pop())
	b := to256(c.top())

	if a.Cmp(b) > 0 {
		b.Set(one)
	} else {
		b.Set(zero)
	}
}

func opSignExtension(c *state) {
	ext := c.pop()
	x := c.top()

	if ext.Cmp(wordSize) > 0 {
		return
	}

	if x == nil {
		return
	}

	bit := uint(ext.Uint64()*8 + 7)

	mask := acquireBig().Set(one)
	mask.Lsh(mask, bit)
	mask.Sub(mask, one)

	if x.Bit(int(bit)) > 0 {
		mask.Not(mask)
		x.Or(x, mask)
	} else {
		x.And(x, mask)
	}

	toU256(x)
	releaseBig(mask)
}

func equalOrOverflowsUint256(b *big.Int) bool {
	return b.BitLen() > 8
}

func opShl(c *state) {
	if !c.config.Constantinople {
		c.exit(errOpCodeNotFound)

		return
	}

	shift := c.pop()
	value := c.top()

	if equalOrOverflowsUint256(shift) {
		value.Set(zero)
	} else {
		value.Lsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

func opShr(c *state) {
	if !c.config.Constantinople {
		c.exit(errOpCodeNotFound)

		return
	}

	shift := c.pop()
	value := c.top()

	if equalOrOverflowsUint256(shift) {
		value.Set(zero)
	} else {
		value.Rsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

func opSar(c *state) {
	if !c.config.Constantinople {
		c.exit(errOpCodeNotFound)

		return
	}

	shift := c.pop()
	value := to256(c.top())

	if equalOrOverflowsUint256(shift) {
		if value.Sign() >= 0 {
			value.Set(zero)
		} else {
			value.Set(tt256m1)
		}
	} else {
		value.Rsh(value, uint(shift.Uint64()))
		toU256(value)
	}
}

// memory operations

var bufPool = sync.Pool{
	New: func() interface{} {
		// Store pointer to avoid heap allocation in caller
		// Please check SA6002 in StaticCheck for details
		buf := make([]byte, 128)

		return &buf
	},
}

func opMload(c *state) {
	offset := c.pop()

	var ok bool
	c.tmp, ok = c.get2(c.tmp[:0], offset, wordSize)

	if !ok {
		return
	}

	c.push1().SetBytes(c.tmp)
}

var (
	_W = bits.UintSize
	_S = _W / 8
)

func opMStore(c *state) {
	offset := c.pop()
	val := c.pop()

	if !c.allocateMemory(offset, wordSize) {
		return
	}

	o := offset.Uint64()
	buf := c.memory[o : o+32]

	i := 32

	// convert big.int to bytes
	// https://golang.org/src/math/big/nat.go#L1284
	for _, d := range val.Bits() {
		for j := 0; j < _S; j++ {
			i--

			buf[i] = byte(d)

			d >>= 8
		}
	}

	// fill the rest of the slot with zeros
	for i > 0 {
		i--

		buf[i] = 0
	}
}

func opMStore8(c *state) {
	offset := c.pop()
	val := c.pop()

	if !c.allocateMemory(offset, one) {
		return
	}

	c.memory[offset.Uint64()] = byte(val.Uint64() & 0xff)
}

// --- storage ---

func opSload(c *state) {
	loc := c.top()

	var gas uint64
	if c.config.Istanbul {
		// eip-1884
		gas = 800
	} else if c.config.EIP150 {
		gas = 200
	} else {
		gas = 50
	}

	if !c.consumeGas(gas) {
		return
	}

	val := c.host.GetStorage(c.msg.Address, bigToHash(loc))
	loc.SetBytes(val.Bytes())
}

func opSStore(c *state) {
	if c.inStaticCall() {
		c.exit(errWriteProtection)

		return
	}

	if c.config.Istanbul && c.gas <= 2300 {
		c.exit(errOutOfGas)

		return
	}

	key := c.popHash()
	val := c.popHash()

	legacyGasMetering := !c.config.Istanbul && (c.config.Petersburg || !c.config.Constantinople)

	status := c.host.SetStorage(c.msg.Address, key, val, c.config)
	cost := uint64(0)

	switch status {
	case runtime.StorageUnchanged:
		if c.config.Istanbul {
			// eip-2200
			cost = 800
		} else if legacyGasMetering {
			cost = 5000
		} else {
			cost = 200
		}

	case runtime.StorageModified:
		cost = 5000

	case runtime.StorageModifiedAgain:
		if c.config.Istanbul {
			// eip-2200
			cost = 800
		} else if legacyGasMetering {
			cost = 5000
		} else {
			cost = 200
		}

	case runtime.StorageAdded:
		cost = 20000

	case runtime.StorageDeleted:
		cost = 5000
	}

	if !c.consumeGas(cost) {
		return
	}
}

const sha3WordGas uint64 = 6

func opSha3(c *state) {
	offset := c.pop()
	length := c.pop()

	var ok bool
	if c.tmp, ok = c.get2(c.tmp[:0], offset, length); !ok {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * sha3WordGas) {
		return
	}

	c.tmp = keccak.Keccak256(c.tmp[:0], c.tmp)

	v := c.push1()
	v.SetBytes(c.tmp)
}

func opPop(c *state) {
	c.pop()
}

// context operations

func opAddress(c *state) {
	c.push1().SetBytes(c.msg.Address.Bytes())
}

func opBalance(c *state) {
	addr, _ := c.popAddr()

	var gas uint64
	if c.config.Istanbul {
		// eip-1884
		gas = 700
	} else if c.config.EIP150 {
		gas = 400
	} else {
		gas = 20
	}

	if !c.consumeGas(gas) {
		return
	}

	c.push1().Set(c.host.GetBalance(addr))
}

func opSelfBalance(c *state) {
	if !c.config.Istanbul {
		c.exit(errOpCodeNotFound)

		return
	}

	c.push1().Set(c.host.GetBalance(c.msg.Address))
}

func opChainID(c *state) {
	if !c.config.Istanbul {
		c.exit(errOpCodeNotFound)

		return
	}

	c.push1().SetUint64(uint64(c.host.GetTxContext().ChainID))
}

func opOrigin(c *state) {
	c.push1().SetBytes(c.host.GetTxContext().Origin.Bytes())
}

func opCaller(c *state) {
	c.push1().SetBytes(c.msg.Caller.Bytes())
}

func opCallValue(c *state) {
	v := c.push1()
	if value := c.msg.Value; value != nil {
		v.Set(value)
	} else {
		v.Set(zero)
	}
}

func min(i, j uint64) uint64 {
	if i < j {
		return i
	}

	return j
}

func opCallDataLoad(c *state) {
	offset := c.top()

	bufPtr := bufPool.Get().(*[]byte)
	buf := *bufPtr
	c.setBytes(buf[:32], c.msg.Input, 32, offset)
	offset.SetBytes(buf[:32])
	bufPool.Put(bufPtr)
}

func opCallDataSize(c *state) {
	c.push1().SetUint64(uint64(len(c.msg.Input)))
}

func opCodeSize(c *state) {
	c.push1().SetUint64(uint64(len(c.code)))
}

func opExtCodeSize(c *state) {
	addr, _ := c.popAddr()

	var gas uint64
	if c.config.EIP150 {
		gas = 700
	} else {
		gas = 20
	}

	if !c.consumeGas(gas) {
		return
	}

	c.push1().SetUint64(uint64(c.host.GetCodeSize(addr)))
}

func opGasPrice(c *state) {
	c.push1().SetBytes(c.host.GetTxContext().GasPrice.Bytes())
}

func opReturnDataSize(c *state) {
	if !c.config.Byzantium {
		c.exit(errOpCodeNotFound)
	} else {
		c.push1().SetUint64(uint64(len(c.returnData)))
	}
}

func opExtCodeHash(c *state) {
	if !c.config.Constantinople {
		c.exit(errOpCodeNotFound)

		return
	}

	address, _ := c.popAddr()

	var gas uint64
	if c.config.Istanbul {
		gas = 700
	} else {
		gas = 400
	}

	if !c.consumeGas(gas) {
		return
	}

	v := c.push1()
	if c.host.Empty(address) {
		v.Set(zero)
	} else {
		v.SetBytes(c.host.GetCodeHash(address).Bytes())
	}
}

func opPC(c *state) {
	c.push1().SetUint64(uint64(c.ip))
}

func opMSize(c *state) {
	c.push1().SetUint64(uint64(len(c.memory)))
}

func opGas(c *state) {
	c.push1().SetUint64(c.gas)
}

func (c *state) setBytes(dst, input []byte, size uint64, dataOffset *big.Int) {
	if !dataOffset.IsUint64() {
		// overflow, copy 'size' 0 bytes to dst
		for i := uint64(0); i < size; i++ {
			dst[i] = 0
		}

		return
	}

	inputSize := uint64(len(input))
	begin := min(dataOffset.Uint64(), inputSize)

	copySize := min(size, inputSize-begin)
	if copySize > 0 {
		copy(dst, input[begin:begin+copySize])
	}

	if size-copySize > 0 {
		dst = dst[copySize:]
		for i := uint64(0); i < size-copySize; i++ {
			dst[i] = 0
		}
	}
}

const copyGas uint64 = 3

func opExtCodeCopy(c *state) {
	address, _ := c.popAddr()
	memOffset := c.pop()
	codeOffset := c.pop()
	length := c.pop()

	if !c.allocateMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	var gas uint64
	if c.config.EIP150 {
		gas = 700
	} else {
		gas = 20
	}

	if !c.consumeGas(gas) {
		return
	}

	code := c.host.GetCode(address)
	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], code, size, codeOffset)
	}
}

func opCallDataCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.allocateMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.msg.Input, size, dataOffset)
	}
}

func opReturnDataCopy(c *state) {
	if !c.config.Byzantium {
		c.exit(errOpCodeNotFound)

		return
	}

	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !dataOffset.IsUint64() {
		c.exit(errReturnDataOutOfBounds)

		return
	}

	// if length is 0, return immediately since no need for the data copying nor memory allocation
	if length.Sign() == 0 || !c.allocateMemory(memOffset, length) {
		return
	}

	ulength := length.Uint64()
	if !c.consumeGas(((ulength + 31) / 32) * copyGas) {
		return
	}

	dataEnd := length.Add(dataOffset, length)
	if !dataEnd.IsUint64() {
		c.exit(errReturnDataOutOfBounds)

		return
	}

	dataEndIndex := dataEnd.Uint64()
	if uint64(len(c.returnData)) < dataEndIndex {
		c.exit(errReturnDataOutOfBounds)

		return
	}

	data := c.returnData[dataOffset.Uint64():dataEndIndex]
	copy(c.memory[memOffset.Uint64():memOffset.Uint64()+ulength], data)
}

func opCodeCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.allocateMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * copyGas) {
		return
	}

	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.code, size, dataOffset)
	}
}

// block information

func opBlockHash(c *state) {
	num := c.top()

	if !num.IsInt64() {
		num.Set(zero)

		return
	}

	n := num.Int64()
	lastBlock := c.host.GetTxContext().Number

	if lastBlock-257 < n && n < lastBlock {
		num.SetBytes(c.host.GetBlockHash(n).Bytes())
	} else {
		num.Set(zero)
	}
}

func opCoinbase(c *state) {
	c.push1().SetBytes(c.host.GetTxContext().Coinbase.Bytes())
}

func opTimestamp(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().Timestamp)
}

func opNumber(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().Number)
}

func opDifficulty(c *state) {
	c.push1().SetBytes(c.host.GetTxContext().Difficulty.Bytes())
}

func opGasLimit(c *state) {
	c.push1().SetInt64(c.host.GetTxContext().GasLimit)
}

func opBaseFee(c *state) {
	if !c.config.London {
		c.exit(errOpCodeNotFound)

		return
	}

	c.push(c.host.GetTxContext().BaseFee)
}

func opSelfDestruct(c *state) {
	if c.inStaticCall() {
		c.exit(errWriteProtection)

		return
	}

	address, _ := c.popAddr()

	// try to remove the gas first
	var gas uint64

	// EIP150 reprice fork
	if c.config.EIP150 {
		gas = 5000

		if c.config.EIP158 {
			// if empty and transfers value
			if c.host.Empty(address) && c.host.GetBalance(c.msg.Address).Sign() != 0 {
				gas += 25000
			}
		} else if !c.host.AccountExists(address) {
			gas += 25000
		}
	}

	if !c.consumeGas(gas) {
		return
	}

	c.host.Selfdestruct(c.msg.Address, address)
	c.Halt()
}

func opJump(c *state) {
	if dest := c.pop(); c.validJumpdest(dest) {
		c.ip = int(dest.Uint64() - 1)
	} else {
		c.exit(errInvalidJump)
	}
}

func opJumpi(c *state) {
	dest := c.pop()
	cond := c.pop()

	if cond.Sign() != 0 {
		if c.validJumpdest(dest) {
			c.ip = int(dest.Uint64() - 1)
		} else {
			c.exit(errInvalidJump)
		}
	}
}

func opJumpDest(c *state) {
}

func opPush(n int) instruction {
	return func(c *state) {
		ins := c.code
		ip := c.ip

		v := c.push1()
		if ip+1+n > len(ins) {
			v.SetBytes(append(ins[ip+1:], make([]byte, n)...))
		} else {
			v.SetBytes(ins[ip+1 : ip+1+n])
		}

		c.ip += n
	}
}

func opDup(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n) {
			c.exit(&runtime.StackUnderflowError{StackLen: c.sp, Required: n})
		} else {
			val := c.peekAt(n)
			c.push1().Set(val)
		}
	}
}

func opSwap(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n + 1) {
			c.exit(&runtime.StackUnderflowError{StackLen: c.sp, Required: n + 1})
		} else {
			c.swap(n)
		}
	}
}

func opLog(size int) instruction {
	size = size - 1

	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errWriteProtection)

			return
		}

		if !c.stackAtLeast(2 + size) {
			c.exit(&runtime.StackUnderflowError{StackLen: c.sp, Required: 2 + size})

			return
		}

		mStart := c.pop()
		mSize := c.pop()

		topics := make([]types.Hash, size)
		for i := 0; i < size; i++ {
			topics[i] = bigToHash(c.pop())
		}

		var ok bool

		c.tmp, ok = c.get2(c.tmp[:0], mStart, mSize)
		if !ok {
			return
		}

		c.host.EmitLog(c.msg.Address, topics, c.tmp)

		if !c.consumeGas(uint64(size) * 375) {
			return
		}

		if !c.consumeGas(mSize.Uint64() * 8) {
			return
		}
	}
}

func opStop(c *state) {
	c.Halt()
}

func opCreate(op OpCode) instruction {
	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errWriteProtection)

			return
		}

		if op == CREATE2 {
			if !c.config.Constantinople {
				c.exit(errOpCodeNotFound)

				return
			}
		}

		// reset the return data
		c.resetReturnData()

		contract, err := c.buildCreateContract(op)
		if err != nil {
			c.push1().Set(zero)

			if contract != nil {
				c.gas += contract.Gas
			}

			return
		}

		if contract == nil {
			return
		}

		contract.Type = runtime.Create

		// Correct call
		result := c.host.Callx(contract, c.host)

		v := c.push1()
		if op == CREATE && c.config.Homestead && errors.Is(result.Err, runtime.ErrCodeStoreOutOfGas) {
			v.Set(zero)
		} else if op == CREATE && result.Failed() && !errors.Is(result.Err, runtime.ErrCodeStoreOutOfGas) {
			v.Set(zero)
		} else if op == CREATE2 && result.Failed() {
			v.Set(zero)
		} else {
			v.SetBytes(contract.Address.Bytes())
		}

		c.gas += result.GasLeft

		if result.Reverted() {
			c.returnData = append(c.returnData[:0], result.ReturnValue...)
		}
	}
}

func opCall(op OpCode) instruction {
	return func(c *state) {
		c.resetReturnData()

		if op == CALL && c.inStaticCall() {
			if val := c.peekAt(3); val != nil && val.BitLen() > 0 {
				c.exit(errWriteProtection)

				return
			}
		}

		if op == DELEGATECALL && !c.config.Homestead {
			c.exit(errOpCodeNotFound)

			return
		}

		if op == STATICCALL && !c.config.Byzantium {
			c.exit(errOpCodeNotFound)

			return
		}

		var callType runtime.CallType

		switch op {
		case CALL:
			callType = runtime.Call

		case CALLCODE:
			callType = runtime.CallCode

		case DELEGATECALL:
			callType = runtime.DelegateCall

		case STATICCALL:
			callType = runtime.StaticCall

		default:
			panic("not expected") //nolint:gocritic
		}

		contract, offset, size, err := c.buildCallContract(op)
		if err != nil {
			c.push1().Set(zero)

			if contract != nil {
				c.gas += contract.Gas
			}

			return
		}

		if contract == nil {
			return
		}

		contract.Type = callType

		result := c.host.Callx(contract, c.host)

		v := c.push1()
		if result.Succeeded() {
			v.Set(one)
		} else {
			v.Set(zero)
		}

		if result.Succeeded() || result.Reverted() {
			if len(result.ReturnValue) != 0 && size > 0 {
				copy(c.memory[offset:offset+size], result.ReturnValue)
			}
		}

		c.gas += result.GasLeft
		c.returnData = append(c.returnData[:0], result.ReturnValue...)
	}
}

func (c *state) buildCallContract(op OpCode) (*runtime.Contract, uint64, uint64, error) {
	// Pop input arguments
	initialGas := c.pop()
	addr, _ := c.popAddr()

	var value *big.Int
	if op == CALL || op == CALLCODE {
		value = c.pop()
	}

	// input range
	inOffset := c.pop()
	inSize := c.pop()

	// output range
	retOffset := c.pop()
	retSize := c.pop()

	// Get the input arguments
	args, ok := c.get2(nil, inOffset, inSize)
	if !ok {
		return nil, 0, 0, nil
	}
	// Check if the memory return offsets are out of bounds
	if !c.allocateMemory(retOffset, retSize) {
		return nil, 0, 0, nil
	}

	var gasCost uint64
	if c.config.EIP150 {
		gasCost = 700
	} else {
		gasCost = 40
	}

	transfersValue := (op == CALL || op == CALLCODE) && value != nil && value.Sign() != 0

	if op == CALL {
		if c.config.EIP158 {
			if transfersValue && c.host.Empty(addr) {
				gasCost += 25000
			}
		} else if !c.host.AccountExists(addr) {
			gasCost += 25000
		}
	}

	if transfersValue {
		gasCost += 9000
	}

	var gas uint64

	ok = initialGas.IsUint64()

	if c.config.EIP150 {
		availableGas := c.gas - gasCost
		availableGas = availableGas - availableGas/64

		if !ok || availableGas < initialGas.Uint64() {
			gas = availableGas
		} else {
			gas = initialGas.Uint64()
		}
	} else {
		if !ok {
			c.exit(errOutOfGas)

			return nil, 0, 0, nil
		}
		gas = initialGas.Uint64()
	}

	gasCostTmp, isOverflow := common.SafeAddUint64(gasCost, gas)
	if isOverflow {
		c.exit(errGasUintOverflow)

		return nil, 0, 0, nil
	}

	gasCost = gasCostTmp

	// Consume gas cost
	if !c.consumeGas(gasCost) {
		return nil, 0, 0, nil
	}

	if transfersValue {
		gas += 2300
	}

	parent := c

	contract := runtime.NewContractCall(
		c.msg.Depth+1,
		parent.msg.Origin,
		parent.msg.Address,
		addr,
		value,
		gas,
		c.host.GetCode(addr),
		args,
	)

	if op == STATICCALL || parent.msg.Static {
		contract.Static = true
	}

	if op == CALLCODE || op == DELEGATECALL {
		contract.Address = parent.msg.Address
		if op == DELEGATECALL {
			contract.Value = parent.msg.Value
			contract.Caller = parent.msg.Caller
		}
	}

	if transfersValue {
		if c.host.GetBalance(c.msg.Address).Cmp(value) < 0 {
			return contract, 0, 0, types.ErrInsufficientFunds
		}
	}

	return contract, retOffset.Uint64(), retSize.Uint64(), nil
}

func (c *state) buildCreateContract(op OpCode) (*runtime.Contract, error) {
	// Pop input arguments
	value := c.pop()
	offset := c.pop()
	length := c.pop()

	var salt *big.Int
	if op == CREATE2 {
		salt = c.pop()
	}

	// check if the value can be transferred
	hasTransfer := value != nil && value.Sign() != 0

	// Calculate and consume gas cost

	// var overflow bool
	var gasCost uint64

	// Both CREATE and CREATE2 use memory
	var input []byte

	var ok bool

	input, ok = c.get2(input[:0], offset, length) // Does the memory check
	if !ok {
		return nil, nil
	}

	// Consume memory resize gas (TODO, change with get2) (to be fixed in EVM-528) //nolint:godox
	if !c.consumeGas(gasCost) {
		return nil, nil
	}

	if hasTransfer {
		if c.host.GetBalance(c.msg.Address).Cmp(value) < 0 {
			return nil, types.ErrInsufficientFunds
		}
	}

	if op == CREATE2 {
		// Consume sha3 gas cost
		size := length.Uint64()
		if !c.consumeGas(((size + 31) / 32) * sha3WordGas) {
			return nil, nil
		}
	}

	// Calculate and consume gas for the call
	gas := c.gas

	// CREATE2 uses by default EIP150
	if c.config.EIP150 || op == CREATE2 {
		gas -= gas / 64
	}

	if !c.consumeGas(gas) {
		return nil, nil
	}

	// Calculate address
	var address types.Address
	if op == CREATE {
		address = crypto.CreateAddress(c.msg.Address, c.host.GetNonce(c.msg.Address))
	} else {
		address = crypto.CreateAddress2(c.msg.Address, bigToHash(salt), input)
	}

	contract := runtime.NewContractCreation(c.msg.Depth+1, c.msg.Origin, c.msg.Address, address, value, gas, input)

	return contract, nil
}

func opHalt(op OpCode) instruction {
	return func(c *state) {
		if op == REVERT && !c.config.Byzantium {
			c.exit(errOpCodeNotFound)

			return
		}

		offset := c.pop()
		size := c.pop()

		var ok bool
		c.ret, ok = c.get2(c.ret[:0], offset, size)

		if !ok {
			return
		}

		if op == REVERT {
			c.exit(errRevert)
		} else {
			c.Halt()
		}
	}
}

var (
	tt256   = new(big.Int).Lsh(big.NewInt(1), 256)   // 2 ** 256
	tt256m1 = new(big.Int).Sub(tt256, big.NewInt(1)) // 2 ** 256 - 1
)

func toU256(x *big.Int) *big.Int {
	if x.Sign() < 0 || x.BitLen() > 256 {
		x.And(x, tt256m1)
	}

	return x
}

func to256(x *big.Int) *big.Int {
	if x.BitLen() > 255 {
		x.Sub(x, tt256)
	}

	return x
}
