package precompiled

import (
	"fmt"

	"github.com/mitchellh/mapstructure"

	"github.com/umbracle/minimal/chain"
)

// Precompiled is the interface for the precompiled contracts
type Precompiled interface {
	Gas(input []byte) uint64
	Call(input []byte) ([]byte, error)
}

// CreatePrecompiled creates a precompiled contract from a builtin genesis reference.
func CreatePrecompiled(b *chain.Builtin) (Precompiled, error) {
	p, ok := BuiltinPrecompiled[b.Name]
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
	return p, nil
}

// Exists returns true if the precompiled contract exists
func Exists(name string) bool {
	_, ok := BuiltinPrecompiled[name]
	return ok
}

// BuiltinPrecompiled are the available precompiled contracts
var BuiltinPrecompiled = map[string]Precompiled{
	"ecrecover":         &ecrecover{},
	"sha256":            &sha256hash{},
	"ripemd160":         &ripemd160hash{},
	"identity":          &dataCopy{},
	"modexp":            &modExp{},
	"alt_bn128_add":     &bn256Add{},
	"alt_bn128_mul":     &bn256ScalarMul{},
	"alt_bn128_pairing": &bn256Pairing{},
}

/*
var ContractsHomestead = map[common.Address]Precompiled{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
}

var ContractsByzantium = map[common.Address]Precompiled{
	common.BytesToAddress([]byte{1}): &ecrecover{},
	common.BytesToAddress([]byte{2}): &sha256hash{},
	common.BytesToAddress([]byte{3}): &ripemd160hash{},
	common.BytesToAddress([]byte{4}): &dataCopy{},
	common.BytesToAddress([]byte{5}): &modExp{},
	common.BytesToAddress([]byte{6}): &bn256Add{},
	common.BytesToAddress([]byte{7}): &bn256ScalarMul{},
	common.BytesToAddress([]byte{8}): &bn256Pairing{},
}
*/
