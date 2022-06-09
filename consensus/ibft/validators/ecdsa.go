package validators

import (
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type ECDSAValidatorSet []types.Address

func ParseECDSAValidators(values []string) (ValidatorSet, error) {
	addrs := make([]types.Address, 0)

	for _, v := range values {
		bytes, err := hex.DecodeString(strings.TrimPrefix(v, "0x"))
		if err != nil {
			return nil, err
		}

		addrs = append(addrs, types.BytesToAddress(bytes))
	}

	newValSet := ECDSAValidatorSet(addrs)

	return &newValSet, nil
}

func ConvertKeysToECDSAValidatorSet(keys []*ecdsa.PrivateKey) *ECDSAValidatorSet {
	addrs := make([]types.Address, len(keys))

	for idx, key := range keys {
		addrs[idx] = crypto.PubKeyToAddress(&key.PublicKey)
	}

	valSet := ECDSAValidatorSet(addrs)

	return &valSet
}

// Len returns the size of the validator set
func (v *ECDSAValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *ECDSAValidatorSet) Equal(tv ValidatorSet) bool {
	target, ok := tv.(*ECDSAValidatorSet)
	if !ok {
		return false
	}

	if len(*v) != len(*target) {
		return false
	}

	for indx := range *v {
		if (*v)[indx] != (*target)[indx] {
			return false
		}
	}

	return true
}

// Copy returns a clone of ECDSAValidatorSet
func (v *ECDSAValidatorSet) Copy() ValidatorSet {
	addrs := make([]types.Address, v.Len())
	copy(addrs, *v)

	clone := ECDSAValidatorSet(addrs)

	return &clone
}

//
func (v *ECDSAValidatorSet) GetAddress(index int) types.Address {
	if index < v.Len() {
		return (*v)[index]
	}

	return types.ZeroAddress
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *ECDSAValidatorSet) Index(target types.Address) int {
	for index, addr := range *v {
		if addr == target {
			return index
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *ECDSAValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *ECDSAValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
	var seed uint64

	if lastProposer == types.ZeroAddress {
		seed = round
	} else {
		offset := 0
		if indx := v.Index(lastProposer); indx != -1 {
			offset = indx
		}

		seed = uint64(offset) + round + 1
	}

	pick := seed % uint64(v.Len())

	return (*v)[pick]
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes (F), based on the current validator set
func (v *ECDSAValidatorSet) MaxFaultyNodes() int {
	return CalcMaxFaultyNodes(v)
}

// Add adds a new address to the validator set
func (v *ECDSAValidatorSet) Add(addr types.Address) {
	*v = append(*v, addr)
}

// Del removes an address from the validator set
func (v *ECDSAValidatorSet) Del(target types.Address) {
	for indx, addr := range *v {
		if addr == target {
			*v = append((*v)[:indx], (*v)[indx+1:]...)
		}
	}
}

// Merge adds set of validators
func (v *ECDSAValidatorSet) Merge(newRawSet ValidatorSet) error {
	newSet, ok := newRawSet.(*ECDSAValidatorSet)
	if !ok {
		return fmt.Errorf("can't merge with ECDSAValidatorSet and %T", newRawSet)
	}

	for _, newVal := range *newSet {
		if !v.Includes(newVal) {
			v.Add(newVal)
		}
	}

	return nil
}

func (v *ECDSAValidatorSet) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*v) == 0 {
		return ar.NewNullArray()
	}

	valSet := ar.NewArray()

	for _, addr := range *v {
		addr := addr

		if len(addr) == 0 {
			valSet.Set(ar.NewNull())
		} else {
			valSet.Set(ar.NewBytes(addr[:]))
		}
	}

	return valSet
}

func (v *ECDSAValidatorSet) UnmarshalRLPFrom(p *fastrlp.Parser, vv *fastrlp.Value) error {
	vals, err := vv.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", vv.Type())
	}

	valSet := make([]types.Address, len(vals))

	for index, val := range vals {
		raw := make([]byte, 0)
		if raw, err = val.GetBytes(raw); err != nil {
			return err
		}

		valSet[index] = types.BytesToAddress(raw)
	}

	(*v) = ECDSAValidatorSet(valSet)

	return nil
}
