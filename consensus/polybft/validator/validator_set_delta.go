package validator

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/umbracle/fastrlp"
)

// ValidatorSetDelta holds information about added and removed validators compared to the previous epoch
type ValidatorSetDelta struct {
	// Added is the slice of added validators
	Added AccountSet
	// Updated is the slice of updated valiadtors
	Updated AccountSet
	// Removed is a bitmap of the validators removed from the set
	Removed bitmap.Bitmap
}

// Equals checks validator set delta equality
func (d *ValidatorSetDelta) Equals(other *ValidatorSetDelta) bool {
	if other == nil {
		return false
	}

	return d.Added.Equals(other.Added) &&
		d.Updated.Equals(other.Updated) &&
		bytes.Equal(d.Removed, other.Removed)
}

// MarshalRLPWith marshals ValidatorSetDelta to RLP format
func (d *ValidatorSetDelta) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	addedValidatorsRaw := ar.NewArray()
	updatedValidatorsRaw := ar.NewArray()

	for _, validatorAccount := range d.Added {
		addedValidatorsRaw.Set(validatorAccount.MarshalRLPWith(ar))
	}

	for _, validatorAccount := range d.Updated {
		updatedValidatorsRaw.Set(validatorAccount.MarshalRLPWith(ar))
	}

	vv.Set(addedValidatorsRaw)         // added
	vv.Set(updatedValidatorsRaw)       // updated
	vv.Set(ar.NewCopyBytes(d.Removed)) // removed

	return vv
}

// UnmarshalRLPWith unmarshals ValidatorSetDelta from RLP format
func (d *ValidatorSetDelta) UnmarshalRLPWith(v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) == 0 {
		return nil
	} else if num := len(elems); num != 3 {
		return fmt.Errorf("incorrect elements count to decode validator set delta, expected 3 but found %d", num)
	}

	// Validators (added)
	{
		validatorsRaw, err := elems[0].GetElems()
		if err != nil {
			return fmt.Errorf("array expected for added validators")
		}

		d.Added, err = unmarshalValidators(validatorsRaw)
		if err != nil {
			return err
		}
	}

	// Validators (updated)
	{
		validatorsRaw, err := elems[1].GetElems()
		if err != nil {
			return fmt.Errorf("array expected for updated validators")
		}

		d.Updated, err = unmarshalValidators(validatorsRaw)
		if err != nil {
			return err
		}
	}

	// Bitmap (removed)
	{
		dst, err := elems[2].GetBytes(nil)
		if err != nil {
			return err
		}

		d.Removed = bitmap.Bitmap(dst)
	}

	return nil
}

// unmarshalValidators unmarshals RLP encoded validators and returns AccountSet instance
func unmarshalValidators(validatorsRaw []*fastrlp.Value) (AccountSet, error) {
	if len(validatorsRaw) == 0 {
		return nil, nil
	}

	validators := make(AccountSet, len(validatorsRaw))

	for i, validatorRaw := range validatorsRaw {
		acc := &ValidatorMetadata{}
		if err := acc.UnmarshalRLPWith(validatorRaw); err != nil {
			return nil, err
		}

		validators[i] = acc
	}

	return validators, nil
}

// IsEmpty returns indication whether delta is empty (namely added, updated slices and removed bitmap are empty)
func (d *ValidatorSetDelta) IsEmpty() bool {
	return len(d.Added) == 0 &&
		len(d.Updated) == 0 &&
		d.Removed.Len() == 0
}

// Copy creates deep copy of ValidatorSetDelta
func (d *ValidatorSetDelta) Copy() *ValidatorSetDelta {
	added := d.Added.Copy()
	removed := make([]byte, len(d.Removed))
	copy(removed, d.Removed)

	return &ValidatorSetDelta{Added: added, Removed: removed}
}

// fmt.Stringer interface implementation
func (d *ValidatorSetDelta) String() string {
	return fmt.Sprintf("Added: \n%v Removed: %v\n Updated: \n%v", d.Added, d.Removed, d.Updated)
}
