package signer

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/umbracle/fastrlp"
)

var (
	// IstanbulDigest represents a hash of "Istanbul practical byzantine fault tolerance"
	// to identify whether the block is from Istanbul consensus engine
	IstanbulDigest = types.StringToHash("0x63746963616c2062797a616e74696e65206661756c7420746f6c6572616e6365")

	// IstanbulExtraVanity represents a fixed number of extra-data bytes reserved for proposer vanity
	IstanbulExtraVanity = 32

	// IstanbulExtraSeal represents the fixed number of extra-data bytes reserved for proposer seal
	IstanbulExtraSeal = 65

	zeroBytes = make([]byte, 32)
)

// IstanbulExtra defines the structure of the extra field for Istanbul
type IstanbulExtra struct {
	Validators           validators.Validators
	ProposerSeal         []byte
	CommittedSeals       Seals
	ParentCommittedSeals Seals
}

type Seals interface {
	// Number of committed seals
	Num() int
	MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}

// MarshalRLPTo defines the marshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(i.MarshalRLPWith, dst)
}

// MarshalRLPWith defines the marshal function implementation for IstanbulExtra
func (i *IstanbulExtra) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// Validators
	vv.Set(i.Validators.MarshalRLPWith(ar))

	// ProposerSeal
	if len(i.ProposerSeal) == 0 {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewCopyBytes(i.ProposerSeal))
	}

	// CommittedSeal
	vv.Set(i.CommittedSeals.MarshalRLPWith(ar))

	// ParentCommittedSeal
	if i.ParentCommittedSeals != nil {
		vv.Set(i.ParentCommittedSeals.MarshalRLPWith(ar))
	}

	return vv
}

// UnmarshalRLP defines the unmarshal function wrapper for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(i.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom defines the unmarshal implementation for IstanbulExtra
func (i *IstanbulExtra) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 3 {
		return fmt.Errorf("incorrect number of elements to decode istambul extra, expected 3 but found %d", len(elems))
	}

	// Validators
	if err := i.Validators.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	// ProposerSeal
	if i.ProposerSeal, err = elems[1].GetBytes(i.ProposerSeal); err != nil {
		return fmt.Errorf("failed to decode Seal: %w", err)
	}

	// CommittedSeal
	if err := i.CommittedSeals.UnmarshalRLPFrom(p, elems[2]); err != nil {
		return err
	}

	// ParentCommitted
	if len(elems) >= 4 && i.ParentCommittedSeals != nil {
		if err := i.ParentCommittedSeals.UnmarshalRLPFrom(p, elems[3]); err != nil {
			return err
		}
	}

	return nil
}

// UnmarshalRLPForParentCS defines the unmarshal function wrapper for IstanbulExtra
// that parses only Parent Committed Seals
func (i *IstanbulExtra) unmarshalRLPForParentCS(input []byte) error {
	return types.UnmarshalRlp(i.unmarshalRLPFromForParentCS, input)
}

// UnmarshalRLPFrom defines the unmarshal implementation for IstanbulExtra
// that parses only Parent Committed Seals
func (i *IstanbulExtra) unmarshalRLPFromForParentCS(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	// ParentCommitted
	if len(elems) >= 4 {
		if err := i.ParentCommittedSeals.UnmarshalRLPFrom(p, elems[3]); err != nil {
			return err
		}
	}

	return nil
}

// putIbftExtra sets the IBFT extra data field into the header
func putIbftExtra(h *types.Header, istanbulExtra *IstanbulExtra) {
	// Pad zeros to the right up to istanbul vanity
	extra := h.ExtraData
	if len(extra) < IstanbulExtraVanity {
		extra = append(extra, zeroBytes[:IstanbulExtraVanity-len(extra)]...)
	} else {
		extra = extra[:IstanbulExtraVanity]
	}

	h.ExtraData = istanbulExtra.MarshalRLPTo(extra)
}

// packFieldsIntoExtra is a helper function
// that injects a few fields into IBFT Extra
// without modifying other fields
// Validators, CommittedSeals, and ParentCommittedSeals have a few types
// and extra must have these instances before unmarshalling usually
// This function doesn't require the field instances that don't update
func packFieldsIntoExtra(
	extraBytes []byte,
	packFn func(
		ar *fastrlp.Arena,
		oldValues []*fastrlp.Value,
		newArrayValue *fastrlp.Value,
	) error,
) []byte {
	extraHeader := extraBytes[:IstanbulExtraVanity]
	extraBody := extraBytes[IstanbulExtraVanity:]

	newExtraBody := types.MarshalRLPTo(func(ar *fastrlp.Arena) *fastrlp.Value {
		vv := ar.NewArray()

		_ = types.UnmarshalRlp(func(p *fastrlp.Parser, v *fastrlp.Value) error {
			elems, err := v.GetElems()
			if err != nil {
				return err
			}

			if len(elems) < 3 {
				return fmt.Errorf("incorrect number of elements to decode istambul extra, expected 3 but found %d", len(elems))
			}

			return packFn(ar, elems, vv)
		}, extraBody)

		return vv
	}, nil)

	return append(
		extraHeader,
		newExtraBody...,
	)
}

// packProposerSealIntoExtra updates only Seal field in Extra
func packProposerSealIntoExtra(
	extraBytes []byte,
	proposerSeal []byte,
) []byte {
	return packFieldsIntoExtra(
		extraBytes,
		func(
			ar *fastrlp.Arena,
			oldValues []*fastrlp.Value,
			newArrayValue *fastrlp.Value,
		) error {
			// Validators
			newArrayValue.Set(oldValues[0])

			// Seal
			newArrayValue.Set(ar.NewBytes(proposerSeal))

			// CommittedSeal
			newArrayValue.Set(oldValues[2])

			// ParentCommittedSeal
			if len(oldValues) >= 4 {
				newArrayValue.Set(oldValues[3])
			}

			return nil
		},
	)
}

// packCommittedSealsIntoExtra updates only CommittedSeal field in Extra
func packCommittedSealsIntoExtra(
	extraBytes []byte,
	committedSeal Seals,
) []byte {
	return packFieldsIntoExtra(
		extraBytes,
		func(
			ar *fastrlp.Arena,
			oldValues []*fastrlp.Value,
			newArrayValue *fastrlp.Value,
		) error {
			// Validators
			newArrayValue.Set(oldValues[0])

			// Seal
			newArrayValue.Set(oldValues[1])

			// CommittedSeal
			newArrayValue.Set(committedSeal.MarshalRLPWith(ar))

			// ParentCommittedSeal
			if len(oldValues) >= 4 {
				newArrayValue.Set(oldValues[3])
			}

			return nil
		},
	)
}
