package evm

import (
	"errors"
	"math/big"
	"strings"

	"sync"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

var statePool = sync.Pool{
	New: func() interface{} {
		return new(state)
	},
}

func acquireState() *state {
	aquiredState, ok := statePool.Get().(*state)
	if !ok {
		return nil
	}

	return aquiredState
}

func releaseState(s *state) {
	s.reset()
	statePool.Put(s)
}

const stackSize = 1024

var (
	errOutOfGas              = runtime.ErrOutOfGas
	errStackUnderflow        = runtime.ErrStackUnderflow
	errStackOverflow         = runtime.ErrStackOverflow
	errRevert                = runtime.ErrExecutionReverted
	errGasUintOverflow       = errors.New("gas uint64 overflow")
	errWriteProtection       = errors.New("write protection")
	errInvalidJump           = errors.New("invalid jump destination")
	errOpCodeNotFound        = errors.New("opcode not found")
	errReturnDataOutOfBounds = errors.New("return data out of bounds")
)

// Instructions is the code of instructions

type state struct {
	ip   int
	code []byte
	tmp  []byte

	host   runtime.Host
	msg    *runtime.Contract // change with msg
	config *chain.ForksInTime

	// memory
	memory      []byte
	lastGasCost uint64

	// stack
	stack []*big.Int
	sp    int

	// remove later
	evm *EVM

	err  error
	stop bool

	gas uint64

	// bitvec bitvec
	bitmap bitmap

	returnData []byte
	ret        []byte
}

func (c *state) reset() {
	c.sp = 0
	c.ip = 0
	c.gas = 0
	c.lastGasCost = 0
	c.stop = false
	c.err = nil

	// reset bitmap
	c.bitmap.reset()

	// reset memory
	for i := range c.memory {
		c.memory[i] = 0
	}

	c.tmp = c.tmp[:0]
	c.ret = c.ret[:0]
	c.code = c.code[:0]
	// c.returnData = c.returnData[:0]
	c.memory = c.memory[:0]
}

func (c *state) validJumpdest(dest *big.Int) bool {
	udest := dest.Uint64()
	if dest.BitLen() >= 63 || udest >= uint64(len(c.code)) {
		return false
	}

	return c.bitmap.isSet(uint(udest))
}

func (c *state) halt() {
	c.stop = true
}

func (c *state) exit(err error) {
	if err == nil {
		panic("cannot stop with none")
	}

	c.stop = true
	c.err = err
}

func (c *state) push(val *big.Int) {
	c.push1().Set(val)
}

func (c *state) push1() *big.Int {
	if len(c.stack) > c.sp {
		c.sp++

		return c.stack[c.sp-1]
	}

	v := big.NewInt(0)
	c.stack = append(c.stack, v)
	c.sp++

	return v
}

func (c *state) stackAtLeast(n int) bool {
	return c.sp >= n
}

func (c *state) popHash() types.Hash {
	return types.BytesToHash(c.pop().Bytes())
}

func (c *state) popAddr() (types.Address, bool) {
	b := c.pop()
	if b == nil {
		return types.Address{}, false
	}

	return types.BytesToAddress(b.Bytes()), true
}

func (c *state) stackSize() int {
	return c.sp
}

func (c *state) top() *big.Int {
	if c.sp == 0 {
		return nil
	}

	return c.stack[c.sp-1]
}

func (c *state) pop() *big.Int {
	if c.sp == 0 {
		return nil
	}

	o := c.stack[c.sp-1]
	c.sp--

	return o
}

func (c *state) peekAt(n int) *big.Int {
	return c.stack[c.sp-n]
}

func (c *state) swap(n int) {
	c.stack[c.sp-1], c.stack[c.sp-n-1] = c.stack[c.sp-n-1], c.stack[c.sp-1]
}

func (c *state) consumeGas(gas uint64) bool {
	if c.gas < gas {
		c.exit(errOutOfGas)

		return false
	}

	c.gas -= gas

	return true
}

func (c *state) resetReturnData() {
	c.returnData = c.returnData[:0]
}

// Run executes the virtual machine
func (c *state) Run() ([]byte, error) {
	var vmerr error

	codeSize := len(c.code)
	for !c.stop {
		if c.ip >= codeSize {
			c.halt()

			break
		}

		op := OpCode(c.code[c.ip])

		inst := dispatchTable[op]
		if inst.inst == nil {
			c.exit(errOpCodeNotFound)

			break
		}
		// check if the depth of the stack is enough for the instruction
		if c.sp < inst.stack {
			c.exit(errStackUnderflow)

			break
		}
		// consume the gas of the instruction
		if !c.consumeGas(inst.gas) {
			c.exit(errOutOfGas)

			break
		}

		// execute the instruction
		inst.inst(c)

		// check if stack size exceeds the max size
		if c.sp > stackSize {
			c.exit(errStackOverflow)

			break
		}
		c.ip++
	}

	if err := c.err; err != nil {
		vmerr = err
	}

	return c.ret, vmerr
}

func (c *state) inStaticCall() bool {
	return c.msg.Static
}

func bigToHash(b *big.Int) types.Hash {
	return types.BytesToHash(b.Bytes())
}

func (c *state) Len() int {
	return len(c.memory)
}

func (c *state) checkMemory(offset, size *big.Int) bool {
	if size.Sign() == 0 {
		return true
	}

	if !offset.IsUint64() || !size.IsUint64() {
		c.exit(errGasUintOverflow)

		return false
	}

	o := offset.Uint64()
	s := size.Uint64()

	if o > 0xffffffffe0 || s > 0xffffffffe0 {
		c.exit(errGasUintOverflow)

		return false
	}

	if newSize, m := o+s, uint64(len(c.memory)); m < newSize {
		w := (newSize + 31) / 32
		newCost := 3*w + w*w/512
		cost := newCost - c.lastGasCost
		c.lastGasCost = newCost

		if !c.consumeGas(cost) {
			c.exit(errOutOfGas)

			return false
		}

		// resize the memory
		c.memory = extendByteSlice(c.memory, int(w*32))
	}

	return true
}

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}

	return b[:needLen]
}

func (c *state) get2(dst []byte, offset, length *big.Int) ([]byte, bool) {
	if length.Sign() == 0 {
		return nil, true
	}

	if !c.checkMemory(offset, length) {
		return nil, false
	}

	o := offset.Uint64()
	l := length.Uint64()

	dst = append(dst, c.memory[o:o+l]...)

	return dst, true
}

func (c *state) Show() string {
	str := []string{}

	for i := 0; i < len(c.memory); i += 16 {
		j := i + 16
		if j > len(c.memory) {
			j = len(c.memory)
		}

		str = append(str, hex.EncodeToHex(c.memory[i:j]))
	}

	return strings.Join(str, "\n")
}
