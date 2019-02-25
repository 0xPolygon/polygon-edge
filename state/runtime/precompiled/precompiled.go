package precompiled

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
)

type Runtime struct {
}

func (r *Runtime) Run(c *runtime.Contract) ([]byte, uint64, error) {
	return nil, 0, nil
}

// Precompiled is a specific precompiled contract
type Precompiled struct {
	ActiveAt uint64
	Backend  Backend
}

// Backend is the execution interface for the precompiled contracts
type Backend interface {
	Gas(input []byte) uint64
	Call(input []byte) ([]byte, error)
}

// CreatePrecompiled creates a precompiled contract from a builtin genesis reference.
func CreatePrecompiled(b *chain.Builtin) (*Precompiled, error) {
	p, ok := builtinPrecompiled[b.Name]
	if !ok {
		return nil, fmt.Errorf("Precompiled contract '%s' does not exists", b.Name)
	}

	var md mapstructure.Metadata
	config := &mapstructure.DecoderConfig{
		Metadata: &md,
		Result:   &p,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(b.Pricing); err != nil {
		return nil, err
	}

	if len(md.Unused) != 0 {
		return nil, fmt.Errorf("Unused keys in pricing '%s': %v", b.Name, md.Unused)
	}
	return &Precompiled{
		Backend:  p,
		ActiveAt: b.ActivateAt,
	}, nil
}

// Exists returns true if the precompiled contract exists
func Exists(name string) bool {
	_, ok := builtinPrecompiled[name]
	return ok
}

// builtinPrecompiled are the available precompiled contracts
var builtinPrecompiled = map[string]Backend{
	"ecrecover":         &ecrecover{},
	"sha256":            &sha256hash{},
	"ripemd160":         &ripemd160hash{},
	"identity":          &dataCopy{},
	"modexp":            &modExp{},
	"alt_bn128_add":     &bn256Add{},
	"alt_bn128_mul":     &bn256ScalarMul{},
	"alt_bn128_pairing": &bn256Pairing{},
}
