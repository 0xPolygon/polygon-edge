package polybft

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"reflect"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/crypto"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/fastrlp"
)

var accountSetABIType = abi.MustNewType(`tuple(address _address, uint256[4] blsKey)[]`)

// ValidatorMetadata represents a validator metadata (its public identity)
type ValidatorMetadata struct {
	Address     types.Address
	BlsKey      *bls.PublicKey
	VotingPower uint64
}

// Equals compares ValidatorMetadata equality
func (a ValidatorMetadata) Equals(b *ValidatorMetadata) bool {
	if b == nil {
		return false
	}

	return a.Address == b.Address && reflect.DeepEqual(a.BlsKey, b.BlsKey)
}

// Copy returns a deep copy of ValidatorMetadata
func (a ValidatorMetadata) Copy() *ValidatorMetadata {
	copiedBlsKey := a.BlsKey.Marshal()
	blsKey, _ := bls.UnmarshalPublicKey(copiedBlsKey)

	return &ValidatorMetadata{
		Address:     types.BytesToAddress(a.Address[:]),
		BlsKey:      blsKey,
		VotingPower: a.VotingPower,
	}
}

// MarshalRLPWith marshals ValidatorMetadata to the RLP format
func (a ValidatorMetadata) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	// Address
	vv.Set(ar.NewBytes(a.Address.Bytes()))
	// BlsKey
	vv.Set(ar.NewCopyBytes(a.BlsKey.Marshal()))
	// VotingPower
	vv.Set(ar.NewBigInt(new(big.Int).SetUint64(a.VotingPower)))

	return vv
}

// UnmarshalRLPWith unmarshals ValidatorMetadata from the RLP format
func (a *ValidatorMetadata) UnmarshalRLPWith(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num != 3 {
		return fmt.Errorf("incorrect elements count to decode validator account, expected 3 but found %d", num)
	}

	// Address
	addressRaw, err := elems[0].GetBytes(nil)
	if err != nil {
		return fmt.Errorf("expected 'Address' field encoded as bytes. Error: %w", err)
	}

	a.Address = types.BytesToAddress(addressRaw)

	// BlsKey
	blsKeyRaw, err := elems[1].GetBytes(nil)
	if err != nil {
		return fmt.Errorf("expected 'BlsKey' encoded as bytes. Error: %w", err)
	}

	blsKey, err := bls.UnmarshalPublicKey(blsKeyRaw)
	if err != nil {
		return fmt.Errorf("failed to unmarshal BLS public key. Error: %w", err)
	}

	a.BlsKey = blsKey

	// VotingPower
	votingPower := new(big.Int)

	err = elems[2].GetBigInt(votingPower)
	if err != nil {
		return fmt.Errorf("expected 'Voting power' encoded as big int. Error: %w", err)
	}

	a.VotingPower = votingPower.Uint64()

	return nil
}

// fmt.Stringer implementation
func (a ValidatorMetadata) String() string {
	return fmt.Sprintf("Address=%v; BLS Key=%v; Voting Power=%d",
		a.Address.String(), hex.EncodeToString(a.BlsKey.Marshal()), a.VotingPower)
}

// AccountSet is a type alias for slice of ValidatorMetadata instances
type AccountSet []*ValidatorMetadata

// GetAddresses aggregates addresses for given AccountSet
func (as AccountSet) GetAddresses() []types.Address {
	res := make([]types.Address, 0, len(as))
	for _, account := range as {
		res = append(res, account.Address)
	}

	return res
}

// GetBlsKeys aggregates public BLS keys for given AccountSet
func (as AccountSet) GetBlsKeys() []*bls.PublicKey {
	res := make([]*bls.PublicKey, 0, len(as))
	for _, account := range as {
		res = append(res, account.BlsKey)
	}

	return res
}

// Len returns length of AccountSet
func (as AccountSet) Len() int {
	return len(as)
}

// ContainsNodeID checks whether ValidatorMetadata with given nodeID is present in the AccountSet
func (as AccountSet) ContainsNodeID(nodeID string) bool {
	for _, validator := range as {
		if validator.Address.String() == nodeID {
			return true
		}
	}

	return false
}

// ContainsAddress checks whether ValidatorMetadata with given address is present in the AccountSet
func (as AccountSet) ContainsAddress(address types.Address) bool {
	return as.Index(address) != -1
}

// Index returns index of the given ValidatorMetadata, identified by address within the AccountSet.
// If given ValidatorMetadata is not present, it returns -1.
func (as AccountSet) Index(addr types.Address) int {
	for indx, validator := range as {
		if validator.Address == addr {
			return indx
		}
	}

	return -1
}

// Copy returns deep copy of AccountSet
func (as AccountSet) Copy() AccountSet {
	copiedAccs := make([]*ValidatorMetadata, as.Len())
	for i, acc := range as {
		copiedAccs[i] = acc.Copy()
	}

	return AccountSet(copiedAccs)
}

// Hash returns hash value of the AccountSet
func (as AccountSet) Hash() (types.Hash, error) {
	abiEncoded, err := accountSetABIType.Encode(as.AsGenericMaps())
	if err != nil {
		return types.ZeroHash, err
	}

	return types.BytesToHash(crypto.Keccak256(abiEncoded)), nil
}

// AsGenericMaps convert AccountSet object to slice of maps, where each key denotes field name mapped to a value
func (as AccountSet) AsGenericMaps() []map[string]interface{} {
	accountSetMaps := make([]map[string]interface{}, len(as))
	for i, v := range as {
		accountSetMaps[i] = map[string]interface{}{
			"_address": v.Address,
			"blsKey":   v.BlsKey.ToBigInt(),
		}
	}

	return accountSetMaps
}

// GetValidatorMetadata tries to retrieve validator account metadata by given address from the account set.
// It returns nil if such account is not found.
func (as AccountSet) GetValidatorMetadata(address types.Address) *ValidatorMetadata {
	i := as.Index(address)
	if i == -1 {
		return nil
	}

	return as[i]
}

// GetFilteredValidators returns filtered validators based on provided bitmap.
// Filtered validators will contain validators whose index corresponds
// to the position in bitmap which has value set to 1.
func (as AccountSet) GetFilteredValidators(bitmap bitmap.Bitmap) (AccountSet, error) {
	var filteredValidators AccountSet
	if len(as) == 0 {
		return filteredValidators, nil
	}

	if bitmap.Len() > uint64(len(as)) {
		for i := len(as); i < int(bitmap.Len()); i++ {
			if bitmap.IsSet(uint64(i)) {
				return filteredValidators, errors.New("invalid bitmap filter provided")
			}
		}
	}

	for i, validator := range as {
		if bitmap.IsSet(uint64(i)) {
			filteredValidators = append(filteredValidators, validator)
		}
	}

	return filteredValidators, nil
}

// ApplyDelta receives ValidatorSetDelta and applies it to the values from the current AccountSet
// (removes the ones marked for deletion and adds the one which are being added by delta)
// Function returns new AccountSet with old and new data merged. AccountSet is immutable!
func (as AccountSet) ApplyDelta(validatorsDelta *ValidatorSetDelta) (AccountSet, error) {
	if validatorsDelta == nil || validatorsDelta.IsEmpty() {
		return as.Copy(), nil
	}

	// Figure out which validators from the existing set are not marked for deletion.
	// Those should be kept in the snapshot.
	validators := make(AccountSet, 0)

	for i, validator := range as {
		// If a validator is not in the Removed set, or it is in the Removed set
		// but it exists in the Added set as well (which should never happen),
		// the validator should remain in the validator set.
		if !validatorsDelta.Removed.IsSet(uint64(i)) ||
			validatorsDelta.Added.ContainsAddress(validator.Address) {
			validators = append(validators, validator)
		}
	}

	// Append added validators
	for _, addedValidator := range validatorsDelta.Added {
		if validators.ContainsAddress(addedValidator.Address) {
			return nil, fmt.Errorf("validator %v is already present in the validators snapshot", addedValidator.Address.String())
		}

		validators = append(validators, addedValidator)
	}

	return validators, nil
}

// Marshal marshals AccountSet to JSON
func (as AccountSet) Marshal() ([]byte, error) {
	return json.Marshal(as)
}

// Unmarshal unmarshals AccountSet from JSON
func (as *AccountSet) Unmarshal(b []byte) error {
	return json.Unmarshal(b, as)
}
