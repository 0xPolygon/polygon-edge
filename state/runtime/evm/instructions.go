package evm

import (
	"math/big"
	"math/bits"
	"sync"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/crypto"
	"github.com/umbracle/minimal/helper"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/types"
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
		b.Set(zero)
		return
	}

	neg := a.Sign() < 0
	b.Mod(a.Abs(a), b.Abs(b))
	if neg {
		b.Neg(b)
	}
	toU256(b)
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

	gas := uint64((y.BitLen()+7)/8) * c.evm.gasTable.ExpByte
	if !c.consumeGas(gas) {
		return
	}

	z := acquireBig().Set(one)

	// https://www.programminglogic.com/fast-exponentiation-algorithms/
	for _, d := range y.Bits() {
		for i := 0; i < W; i++ {
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
		// divison by zero
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
		// divison by zero
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
	if !c.evm.config.Constantinople {
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
	if !c.evm.config.Constantinople {
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
	if !c.evm.config.Constantinople {
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
		return make([]byte, 128)
	},
}

func opMload(c *state) {
	offset := c.pop()

	buf := bufPool.Get().([]byte)

	var ok bool
	buf, ok = c.get2(buf[:0], offset, wordSize)
	if !ok {
		return
	}

	c.push1().SetBytes(buf)
	bufPool.Put(buf)
}

var (
	W = bits.UintSize
	S = W / 8
)

func opMStore(c *state) {
	offset := c.pop()
	val := c.pop()

	if !c.checkMemory(offset, wordSize) {
		return
	}

	o := offset.Uint64()
	buf := c.memory[o : o+32]

	i := 32

	// convert big.int to bytes
	// https://golang.org/src/math/big/nat.go#L1284
	for _, d := range val.Bits() {
		for j := 0; j < S; j++ {
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

	if !c.checkMemory(offset, one) {
		return
	}
	c.memory[offset.Uint64()] = byte(val.Uint64() & 0xff)
}

// --- storage ---

func opSload(c *state) {
	loc := c.top()

	if !c.consumeGas(c.evm.gasTable.SLoad) {
		return
	}

	val := c.evm.state.GetState(c.address, bigToHash(loc))
	loc.SetBytes(val.Bytes())
}

func opSStore(c *state) {
	if c.inStaticCall() {
		c.exit(errReadOnly)
		return
	}

	address := c.address

	loc, val := c.pop(), c.pop()

	var gas uint64

	current := c.evm.state.GetState(address, bigToHash(loc))

	// discount gas (constantinople)
	if c.evm.config.Petersburg || !c.evm.config.Constantinople {
		switch {
		case current == (types.Hash{}) && val.Sign() != 0: // 0 => non 0
			gas = SstoreSetGas
		case current != (types.Hash{}) && val.Sign() == 0: // non 0 => 0
			c.evm.state.AddRefund(SstoreRefundGas)
			gas = SstoreClearGas
		default: // non 0 => non 0 (or 0 => 0)
			gas = SstoreResetGas
		}
	} else {
		getGas := func() uint64 {
			// non constantinople gas
			value := bigToHash(val)
			if current == value { // noop (1)
				return NetSstoreNoopGas
			}
			original := c.evm.state.GetCommittedState(address, bigToHash(loc))
			if original == current {
				if original == (types.Hash{}) { // create slot (2.1.1)
					return NetSstoreInitGas
				}
				if value == (types.Hash{}) { // delete slot (2.1.2b)
					c.evm.state.AddRefund(NetSstoreClearRefund)
				}
				return NetSstoreCleanGas // write existing slot (2.1.2)
			}
			if original != (types.Hash{}) {
				if current == (types.Hash{}) { // recreate slot (2.2.1.1)
					c.evm.state.SubRefund(NetSstoreClearRefund)
				} else if value == (types.Hash{}) { // delete slot (2.2.1.2)
					c.evm.state.AddRefund(NetSstoreClearRefund)
				}
			}
			if original == value {
				if original == (types.Hash{}) { // reset to original inexistent slot (2.2.2.1)
					c.evm.state.AddRefund(NetSstoreResetClearRefund)
				} else { // reset to original existing slot (2.2.2.2)
					c.evm.state.AddRefund(NetSstoreResetRefund)
				}
			}
			return NetSstoreDirtyGas
		}
		gas = getGas()
	}

	if !c.consumeGas(gas) {
		return
	}

	c.evm.state.SetState(address, bigToHash(loc), bigToHash(val))
}

func opSha3(c *state) {
	offset := c.pop()
	length := c.pop()

	var ok bool
	if c.tmp, ok = c.get2(c.tmp[:0], offset, length); !ok {
		return
	}

	hash := types.BytesToHash(crypto.Keccak256(c.tmp))

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * Sha3WordGas) {
		return
	}

	v := c.push1()
	v.SetBytes(hash.Bytes())
}

func opPop(c *state) {
	c.pop()
}

// context operations

func opAddress(c *state) {
	c.push1().SetBytes(c.address.Bytes())
}

func opBalance(c *state) {
	addr, _ := c.popAddr()

	if !c.consumeGas(c.evm.gasTable.Balance) {
		return
	}

	c.push1().Set(c.evm.state.GetBalance(addr))
}

func opOrigin(c *state) {
	c.push1().SetBytes(c.origin.Bytes())
}

func opCaller(c *state) {
	c.push1().SetBytes(c.caller.Bytes())
}

func opCallValue(c *state) {
	v := c.push1()
	if value := c.value; value != nil {
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

	buf := bufPool.Get().([]byte)
	c.setBytes(buf[:32], c.input, 32, offset)
	offset.SetBytes(buf[:32])
	bufPool.Put(buf)
}

func opCallDataSize(c *state) {
	c.push1().SetUint64(uint64(len(c.input)))
}

func opCodeSize(c *state) {
	c.push1().SetUint64(uint64(len(c.code)))
}

func opExtCodeSize(c *state) {
	addr, _ := c.popAddr()

	if !c.consumeGas(c.evm.gasTable.ExtcodeSize) {
		return
	}

	c.push1().SetUint64(uint64(c.evm.state.GetCodeSize(addr)))
}

func opGasPrice(c *state) {
	c.push1().Set(c.evm.env.GasPrice)
}

func opReturnDataSize(c *state) {
	if !c.evm.config.Byzantium {
		c.exit(errOpCodeNotFound)
	} else {
		c.push1().SetUint64(uint64(len(c.returnData)))
	}
}

func opExtCodeHash(c *state) {
	if !c.evm.config.Constantinople {
		c.exit(errOpCodeNotFound)
		return
	}

	address, _ := c.popAddr()

	if !c.consumeGas(c.evm.gasTable.ExtcodeHash) {
		return
	}

	v := c.push1()
	if c.evm.state.Empty(address) {
		v.Set(zero)
	} else {
		v.SetBytes(c.evm.state.GetCodeHash(address).Bytes())
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

func opExtCodeCopy(c *state) {
	address, _ := c.popAddr()
	memOffset := c.pop()
	codeOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * CopyGas) {
		return
	}
	if !c.consumeGas(c.evm.gasTable.ExtcodeCopy) {
		return
	}

	code := c.evm.state.GetCode(address)
	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], code, size, codeOffset)
	}
}

func opCallDataCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * CopyGas) {
		return
	}

	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.input, size, dataOffset)
	}
}

func opReturnDataCopy(c *state) {
	if !c.evm.config.Byzantium {
		c.exit(errOpCodeNotFound)
		return
	}

	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * CopyGas) {
		return
	}

	end := length.Add(dataOffset, length)
	if !end.IsUint64() {
		c.exit(errOutOfGas)
		return
	}
	size = end.Uint64()
	if uint64(len(c.returnData)) < size {
		c.exit(errReturnBadSize)
		return
	}

	data := c.returnData[dataOffset.Uint64():size]
	copy(c.memory[memOffset.Uint64():], data)
}

func opCodeCopy(c *state) {
	memOffset := c.pop()
	dataOffset := c.pop()
	length := c.pop()

	if !c.checkMemory(memOffset, length) {
		return
	}

	size := length.Uint64()
	if !c.consumeGas(((size + 31) / 32) * CopyGas) {
		return
	}
	if size != 0 {
		c.setBytes(c.memory[memOffset.Uint64():], c.code, size, dataOffset)
	}
}

// block information

func opBlockHash(c *state) {
	num := c.top()

	if !num.IsUint64() {
		num.Set(zero)
		return
	}

	n := num.Uint64()
	lastBlock := c.evm.env.Number

	var ok bool
	if lastBlock <= 257 {
		ok = true
	} else {
		ok = n > lastBlock-257
	}

	if n < lastBlock && ok {
		num.SetBytes(c.evm.getHash(num.Uint64()).Bytes())
	} else {
		num.Set(zero)
	}
}

func opCoinbase(c *state) {
	c.push1().SetBytes(c.evm.env.Coinbase.Bytes())
}

func opTimestamp(c *state) {
	c.push1().SetUint64(c.evm.env.Timestamp)
}

func opNumber(c *state) {
	c.push1().SetUint64(c.evm.env.Number)
}

func opDifficulty(c *state) {
	v := c.push1()
	v.Set(c.evm.env.Difficulty)
	toU256(v)
}

func opGasLimit(c *state) {
	v := c.push1()
	v.Set(c.evm.env.GasLimit)
	toU256(v)
}

func opSelfDestruct(c *state) {
	if c.inStaticCall() {
		c.exit(errReadOnly)
		return
	}

	address, _ := c.popAddr()

	// try to remove the gas first
	var gas uint64

	// EIP150 homestead gas reprice fork:
	if c.evm.config.EIP150 {
		gas = c.evm.gasTable.Suicide

		if c.evm.config.EIP158 {
			// if empty and transfers value
			if c.evm.state.Empty(address) && c.evm.state.GetBalance(c.address).Sign() != 0 {
				gas += c.evm.gasTable.CreateBySuicide
			}
		} else if !c.evm.state.Exist(address) {
			gas += c.evm.gasTable.CreateBySuicide
		}
	}

	if !c.consumeGas(gas) {
		return
	}

	if !c.evm.state.HasSuicided(c.address) {
		c.evm.state.AddRefund(SuicideRefundGas)
	}

	balance := c.evm.state.GetBalance(c.address)
	c.evm.state.AddBalance(address, balance)
	c.evm.state.Suicide(c.address)

	c.halt()
}

func opJump(c *state) {
	dest := c.pop()

	if c.validJumpdest(dest) {
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
			v.SetBytes(helper.RightPadBytes(ins[ip+1:len(ins)], n))
		} else {
			v.SetBytes(ins[ip+1 : ip+1+n])
		}

		c.ip += n
	}
}

func opDup(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n) {
			c.exit(errStackUnderflow)
		} else {
			val := c.peekAt(n)
			c.push1().Set(val)
		}
	}
}

func opSwap(n int) instruction {
	return func(c *state) {
		if !c.stackAtLeast(n + 1) {
			c.exit(errStackUnderflow)
		} else {
			c.swap(n)
		}
	}
}

func opLog(size int) instruction {
	size = size - 1
	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errReadOnly)
			return
		}

		if !c.stackAtLeast(2 + size) {
			c.exit(errStackUnderflow)
			return
		}

		mStart := c.pop()
		mSize := c.pop()

		topics := make([]types.Hash, size)
		for i := 0; i < size; i++ {
			topics[i] = bigToHash(c.pop())
		}

		log := &types.Log{
			Address:     c.address,
			Topics:      topics,
			BlockNumber: c.evm.env.Number,
		}

		var ok bool
		log.Data, ok = c.get2(log.Data[:0], mStart, mSize)
		if !ok {
			return
		}

		c.evm.state.AddLog(log)
		if !c.consumeGas(uint64(size) * LogTopicGas) {
			return
		}
		if !c.consumeGas(mSize.Uint64() * LogDataGas) {
			return
		}
	}
}

func opStop(c *state) {
	c.halt()
}

func opCreate(op OpCode) instruction {
	return func(c *state) {
		if c.inStaticCall() {
			c.exit(errReadOnly)
			return
		}

		if op == CREATE2 {
			if !c.evm.config.Constantinople {
				c.exit(errOpCodeNotFound)
				return
			}
		}

		contract := c.buildCreateContract(op)
		if contract == nil {
			return
		}

		ret, gas, err := c.evm.executor.Create(contract)

		v := c.push1()
		if op == CREATE && c.evm.config.Homestead && err == runtime.ErrCodeStoreOutOfGas {
			v.Set(zero)
		} else if err != nil && err != runtime.ErrCodeStoreOutOfGas {
			v.Set(zero)
		} else {
			v.SetBytes(contract.Address.Bytes())
		}

		c.gas += gas
		if err == runtime.ErrExecutionReverted {
			c.returnData = ret
		} else {
			c.returnData = nil
		}
	}
}

func opCall(op OpCode) instruction {
	return func(c *state) {
		c.returnData = nil

		if op == CALL && c.inStaticCall() {
			if val := c.peekAt(3); val != nil && val.BitLen() > 0 {
				c.exit(errReadOnly)
				return
			}
		}

		if op == DELEGATECALL && !c.evm.config.Homestead {
			c.exit(errOpCodeNotFound)
			return
		}
		if op == STATICCALL && !c.evm.config.Byzantium {
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
			panic("not expected")
		}

		contract := c.buildCallContract(op)
		if contract == nil {
			return
		}

		ret, gas, err := c.evm.executor.Call(contract, callType)

		v := c.push1()
		if err != nil {
			v.Set(zero)
		} else {
			v.Set(one)
		}

		if err == nil || err == runtime.ErrExecutionReverted {
			// TODO, change retOffset
			// c.Set(contract.RetOffset, contract.RetSize, ret)
			if len(ret) != 0 {
				offset := contract.RetOffset
				copy(c.memory[offset:offset+contract.RetSize], ret)
			}
		}

		c.gas += gas
		c.returnData = ret
	}
}

func (c *state) buildCallContract(op OpCode) *runtime.Contract {
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

	// Calculate and consume gas cost

	// Memory cost needs to consider both input and output resizes (HACK)
	in := calcMemSize(inOffset, inSize)
	ret := calcMemSize(retOffset, retSize)

	max := in
	if in.Cmp(ret) < 0 {
		max = ret
	}
	if !max.IsUint64() {
		c.exit(errOutOfGas)
		return nil
	}
	if !c.checkMemory(zero, max) {
		return nil
	}

	args := c.Get(inOffset, inSize)

	gasCost := c.evm.gasTable.Calls
	eip158 := c.evm.config.EIP158
	transfersValue := value != nil && value.Sign() != 0

	if op == CALL {
		if eip158 {
			if transfersValue && c.evm.state.Empty(addr) {
				gasCost += CallNewAccountGas
			}
		} else if !c.evm.state.Exist(addr) {
			gasCost += CallNewAccountGas
		}
	}
	if op == CALL || op == CALLCODE {
		if transfersValue {
			gasCost += CallValueTransferGas
		}
	}

	gas, ok := callGas(c.evm.gasTable, c.gas, gasCost, initialGas)
	if !ok {
		c.exit(errOutOfGas)
		return nil
	}

	gasCost = gasCost + gas

	// Consume gas cost
	if !c.consumeGas(gasCost) {
		return nil
	}

	if op == CALL || op == CALLCODE {
		if transfersValue {
			gas += CallStipend
		}
	}

	parent := c

	contract := runtime.NewContractCall(c.depth+1, parent.origin, parent.address, addr, value, gas, c.evm.state.GetCode(addr), args)

	contract.RetOffset = retOffset.Uint64()
	contract.RetSize = retSize.Uint64()

	if op == STATICCALL || parent.static {
		contract.Static = true
	}
	if op == CALLCODE || op == DELEGATECALL {
		contract.Address = parent.address
		if op == DELEGATECALL {
			contract.Value = parent.value
			contract.Caller = parent.caller
		}
	}

	return contract
}

func callGas(gasTable chain.GasTable, availableGas, base uint64, callCost *big.Int) (uint64, bool) {
	if gasTable.CreateBySuicide > 0 {
		availableGas = availableGas - base
		gas := availableGas - availableGas/64
		// If the bit length exceeds 64 bit we know that the newly calculated "gas" for EIP150
		// is smaller than the requested amount. Therefor we return the new gas instead
		// of returning an error.
		if callCost.BitLen() > 64 || gas < callCost.Uint64() {
			return gas, true
		}
	}
	if !callCost.IsUint64() {
		return 0, false
	}

	return callCost.Uint64(), true
}

func (c *state) buildCreateContract(op OpCode) *runtime.Contract {
	// Pop input arguments
	value := c.pop()
	offset := c.pop()
	length := c.pop()

	var salt *big.Int
	if op == CREATE2 {
		salt = c.pop()
	}

	// Calculate and consume gas cost

	// var overflow bool
	var gasCost uint64

	if !c.checkMemory(offset, length) {
		return nil
	}

	// Both CREATE and CREATE2 use memory
	input := c.Get(offset, length)

	// Consume memory resize gas (TODO, change with get2)
	if !c.consumeGas(gasCost) {
		return nil
	}

	if op == CREATE2 {
		// Consume sha3 gas cost
		size := length.Uint64()
		if !c.consumeGas(((size + 31) / 32) * Sha3WordGas) {
			return nil
		}
	}

	// Calculate and consume gas for the call
	gas := c.gas

	// CREATE2 uses by default EIP150
	if c.evm.config.EIP150 || op == CREATE2 {
		gas -= gas / 64
	}

	if !c.consumeGas(gas) {
		return nil
	}

	// Calculate address
	var address types.Address
	if op == CREATE {
		address = crypto.CreateAddress(c.address, c.evm.state.GetNonce(c.address))
	} else {
		address = crypto.CreateAddress2(c.address, bigToHash(salt), input)
	}

	contract := runtime.NewContractCreation(c.depth+1, c.origin, c.address, address, value, gas, input)
	return contract
}

func opHalt(op OpCode) instruction {
	return func(c *state) {
		if op == REVERT && !c.evm.config.Byzantium {
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
			c.halt()
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
