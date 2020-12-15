package precompiled

import (
	"encoding/binary"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/types"
)

var _ runtime.Runtime = &Precompiled{}

type contract interface {
	gas(input []byte, config *chain.ForksInTime) uint64
	run(input []byte) ([]byte, error)
}

// Precompiled is the runtime for the precompiled contracts
type Precompiled struct {
	buf       []byte
	contracts map[types.Address]contract
}

// NewPrecompiled creates a new runtime for the precompiled contracts
func NewPrecompiled() *Precompiled {
	p := &Precompiled{}
	p.setupContracts()
	return p
}

func (p *Precompiled) setupContracts() {
	p.register("1", &ecrecover{p})
	p.register("2", &sha256h{})
	p.register("3", &ripemd160h{p})
	p.register("4", &identity{})

	// Byzantium fork
	p.register("5", &modExp{p})
	p.register("6", &bn256Add{p})
	p.register("7", &bn256Mul{p})
	p.register("8", &bn256Pairing{p})

	// Istanbul fork
	p.register("9", &blake2f{p})
}

func (p *Precompiled) register(addrStr string, b contract) {
	if len(p.contracts) == 0 {
		p.contracts = map[types.Address]contract{}
	}
	p.contracts[types.StringToAddress(addrStr)] = b
}

var (
	five  = types.StringToAddress("5")
	six   = types.StringToAddress("6")
	seven = types.StringToAddress("7")
	eight = types.StringToAddress("8")
	nine  = types.StringToAddress("9")
)

// CanRun implements the runtime interface
func (p *Precompiled) CanRun(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) bool {
	//fmt.Println("-- can run --")
	//fmt.Println(config)
	//fmt.Println(config.Byzantium)

	if _, ok := p.contracts[c.CodeAddress]; !ok {
		return false
	}

	// byzantium precompiles
	switch c.CodeAddress {
	case five:
		fallthrough
	case six:
		fallthrough
	case seven:
		fallthrough
	case eight:
		return config.Byzantium
	}

	// istanbul precompiles
	switch c.CodeAddress {
	case nine:
		return config.Istanbul
	}

	return true
}

// Name implements the runtime interface
func (p *Precompiled) Name() string {
	return "precompiled"
}

// Run runs an execution
func (p *Precompiled) Run(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) ([]byte, uint64, error) {
	contract := p.contracts[c.CodeAddress]
	gasCost := contract.gas(c.Input, config)

	if c.Gas < gasCost {
		return nil, 0, runtime.ErrGasOverflow
	}

	c.Gas = c.Gas - gasCost
	ret, err := contract.run(c.Input)
	if err != nil {
		return nil, 0, err
	}
	return ret, c.Gas, err
}

var zeroPadding = make([]byte, 64)

func (p *Precompiled) leftPad(buf []byte, n int) []byte {
	// TODO, avoid buffer allocation
	l := len(buf)
	if l > n {
		return buf
	}

	tmp := make([]byte, n)
	copy(tmp[n-l:], buf)
	return tmp
}

func (p *Precompiled) get(input []byte, size int) ([]byte, []byte) {
	p.buf = extendByteSlice(p.buf, size)
	n := size
	if len(input) < n {
		n = len(input)
	}

	// copy the part from the input
	copy(p.buf[0:], input[:n])

	// copy empty values
	if n < size {
		rest := size - n
		if rest < 64 {
			copy(p.buf[n:], zeroPadding[0:size-n])
		} else {
			copy(p.buf[n:], make([]byte, rest))
		}
	}
	return p.buf, input[n:]
}

func (p *Precompiled) getUint64(input []byte) (uint64, []byte) {
	p.buf, input = p.get(input, 32)
	num := binary.BigEndian.Uint64(p.buf[24:32])
	return num, input
}

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}
