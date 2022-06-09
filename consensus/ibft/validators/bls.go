package validators

import (
	"bytes"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type BLSPubKey []byte

func (k BLSPubKey) String() string {
	return hex.EncodeToHex(k[:])
}

func (k BLSPubKey) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

func (k *BLSPubKey) Scan(src interface{}) error {
	stringVal, ok := src.([]byte)
	if !ok {
		return errors.New("invalid type assert")
	}

	x, err := hex.DecodeHex(string(stringVal))
	if err != nil {
		return err
	}

	copy(*k, x)

	return nil
}

type BLSValidator struct {
	Address   types.Address
	BLSPubKey BLSPubKey
}

func (v *BLSValidator) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	val := ar.NewArray()

	val.Set(ar.NewBytes(v.Address[:]))

	if len(v.BLSPubKey) == 0 {
		val.Set(ar.NewNull())
	} else {
		val.Set(ar.NewBytes(v.BLSPubKey))
	}

	return val
}

func (v *BLSValidator) UnmarshalRLPFrom(p *fastrlp.Parser, vv *fastrlp.Value) error {
	vals, err := vv.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for CommittedSeal, expected list but found %s", vv.Type())
	}

	{
		raw := make([]byte, 0)
		if raw, err = vals[0].GetBytes(raw); err != nil {
			return err
		}

		v.Address = types.BytesToAddress(raw)
	}

	{
		v.BLSPubKey = make([]byte, 0)
		if v.BLSPubKey, err = vals[1].GetBytes(v.BLSPubKey); err != nil {
			return err
		}
	}

	return nil
}

func (v *BLSValidator) Equals(t *BLSValidator) bool {
	if v.Address != t.Address {
		return false
	}

	if !bytes.Equal(v.BLSPubKey, t.BLSPubKey) {
		return false
	}

	return true
}

type BLSValidatorSet []BLSValidator

func ParseBLSValidators(values []string) (ValidatorSet, error) {
	vals := make([]BLSValidator, 0)

	for _, value := range values {
		subValues := strings.Split(value, ":")

		if len(subValues) != 2 {
			return nil, fmt.Errorf("invalid validator format, expected [Validator Address]:[BLS Public Key]")
		}

		addrBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[0], "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse address: %w", err)
		}

		pubKeyBytes, err := hex.DecodeString(strings.TrimPrefix(subValues[1], "0x"))
		if err != nil {
			return nil, fmt.Errorf("failed to parse BLS Public Key: %w", err)
		}

		vals = append(vals, BLSValidator{
			Address:   types.BytesToAddress(addrBytes),
			BLSPubKey: pubKeyBytes,
		})
	}

	newValSet := BLSValidatorSet(vals)

	return &newValSet, nil
}

func ConvertKeysToBLSValidatorSet(keys []*ecdsa.PrivateKey) (*BLSValidatorSet, error) {
	vals := make([]BLSValidator, len(keys))

	for idx, key := range keys {
		pubkey, err := crypto.ECDSAToBLSPubkey(key)
		if err != nil {
			return nil, err
		}

		vals[idx] = BLSValidator{
			Address:   crypto.PubKeyToAddress(&key.PublicKey),
			BLSPubKey: pubkey,
		}
	}

	valSet := BLSValidatorSet(vals)

	return &valSet, nil
}

// Len returns the size of the validator set
func (v *BLSValidatorSet) Len() int {
	return len(*v)
}

// Equal checks if 2 validator sets are equal
func (v *BLSValidatorSet) Equal(tv ValidatorSet) bool {
	target, ok := tv.(*BLSValidatorSet)
	if !ok {
		return false
	}

	if len(*v) != len(*target) {
		return false
	}

	for indx := range *v {
		x, y := (*v)[indx], (*target)[indx]

		if !x.Equals(&y) {
			return false
		}
	}

	return true
}

// Copy returns a clone of ECDSAValidatorSet
func (v *BLSValidatorSet) Copy() ValidatorSet {
	vals := make([]BLSValidator, v.Len())

	for i := range vals {
		vals[i].Address = (*v)[i].Address
		vals[i].BLSPubKey = make([]byte, len((*v)[i].BLSPubKey))
		copy(vals[i].BLSPubKey, (*v)[i].BLSPubKey)
	}

	clone := BLSValidatorSet(vals)

	return &clone
}

//
func (v *BLSValidatorSet) GetAddress(index int) types.Address {
	if index < v.Len() {
		return (*v)[index].Address
	}

	return types.ZeroAddress
}

// Index returns the index of the passed in address in the validator set.
// Returns -1 if not found
func (v *BLSValidatorSet) Index(target types.Address) int {
	for index, addr := range *v {
		if addr.Address == target {
			return index
		}
	}

	return -1
}

// Includes checks if the address is in the validator set
func (v *BLSValidatorSet) Includes(addr types.Address) bool {
	return v.Index(addr) != -1
}

// CalcProposer calculates the address of the next proposer, from the validator set
func (v *BLSValidatorSet) CalcProposer(round uint64, lastProposer types.Address) types.Address {
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

	return (*v)[pick].Address
}

// MaxFaultyNodes returns the maximum number of allowed faulty nodes (F), based on the current validator set
func (v *BLSValidatorSet) MaxFaultyNodes() int {
	return CalcMaxFaultyNodes(v)
}

// Add adds a new address to the validator set
func (v *BLSValidatorSet) Add(addr types.Address) {
	// TODO: doesn't support right now
}

// Del removes an address from the validator set
func (v *BLSValidatorSet) Del(target types.Address) {
	// TODO: doesn't support right now
}

// Merge adds set of validators
func (v *BLSValidatorSet) Merge(newRawSet ValidatorSet) error {
	newSet, ok := newRawSet.(*BLSValidatorSet)
	if !ok {
		return fmt.Errorf("can't merge with BLSValidatorSet and %T", newRawSet)
	}

	for _, newVal := range *newSet {
		if !v.Includes(newVal.Address) {
			*v = append(*v, newVal)
		}
	}

	return nil
}

func (v *BLSValidatorSet) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	if len(*v) == 0 {
		return ar.NewNullArray()
	}

	valSet := ar.NewArray()

	for _, val := range *v {
		val := val
		valSet.Set(val.MarshalRLPWith(ar))
	}

	return valSet
}

func (v *BLSValidatorSet) UnmarshalRLPFrom(p *fastrlp.Parser, vv *fastrlp.Value) error {
	vals, err := vv.GetElems()
	if err != nil {
		return fmt.Errorf("mismatch of RLP type for ECDSAValidatorSet, expected list but found %s", vv.Type())
	}

	valSet := make([]BLSValidator, len(vals))

	for index := range vals {
		vals := vals
		if err := valSet[index].UnmarshalRLPFrom(p, vals[index]); err != nil {
			return err
		}
	}

	(*v) = BLSValidatorSet(valSet)

	return nil
}
