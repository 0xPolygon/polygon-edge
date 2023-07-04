package precompiled

import (
	"encoding/binary"
	"log"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

var _ runtime.Runtime = &Precompiled{}

var (
	abiBoolTrue, abiBoolFalse []byte
)

func init() {
	// abiBoolTrue is ABI encoded true boolean value
	encodedBool, err := abi.MustNewType("bool").Encode(true)
	if err != nil {
		log.Fatal(err)
	}

	abiBoolTrue = encodedBool

	// abiBoolFalse is ABI encoded false boolean value
	encodedBool, err = abi.MustNewType("bool").Encode(false)
	if err != nil {
		log.Fatal(err)
	}

	abiBoolFalse = encodedBool
}

type contract interface {
	gas(input []byte, config *chain.ForksInTime) uint64
	run(input []byte, caller types.Address, host runtime.Host) ([]byte, error)
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

	// Native transfer precompile
	p.register(contracts.NativeTransferPrecompile.String(), &nativeTransfer{})

	// Console precompile
	// p.register(contracts.ConsolePrecompile.String(), &console{})

	// BLS aggregated signatures verification precompile
	p.register(contracts.BLSAggSigsVerificationPrecompile.String(), &blsAggSignsVerification{})
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
func (p *Precompiled) CanRun(c *runtime.Contract, _ runtime.Host, config *chain.ForksInTime) bool {
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
func (p *Precompiled) Run(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) *runtime.ExecutionResult {
	contract := p.contracts[c.CodeAddress]
	gasCost := contract.gas(c.Input, config)

	// In the case of not enough gas for precompiled execution we return ErrOutOfGas
	if c.Gas < gasCost {
		return &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrOutOfGas,
		}
	}

	c.Gas = c.Gas - gasCost
	returnValue, err := contract.run(c.Input, c.Caller, host)

	result := &runtime.ExecutionResult{
		ReturnValue: returnValue,
		GasLeft:     c.Gas,
		Err:         err,
	}

	if result.Failed() {
		result.GasLeft = 0
		result.ReturnValue = nil
	}

	return result
}

var zeroPadding = make([]byte, 64)

func (p *Precompiled) leftPad(buf []byte, n int) []byte {
	l := len(buf)
	if l > n {
		return buf
	}

	tmp := make([]byte, n)
	copy(tmp[n-l:], buf)

	return tmp
}

func (p *Precompiled) get(input []byte, size int) ([]byte, []byte) {
	p.buf = common.ExtendByteSlice(p.buf, size)
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
