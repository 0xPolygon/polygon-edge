package signer

import (
	"encoding/binary"
	"errors"
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

	errRoundNumberOverflow = errors.New("round number is out of range for 64bit")
)

// IstanbulExtra defines the structure of the extra field for Istanbul
type IstanbulExtra struct {
	Validators           validators.Validators
	ProposerSeal         []byte
	CommittedSeals       Seals
	ParentCommittedSeals Seals
	RoundNumber          *uint64
}

type Seals interface {
	// Number of committed seals
	Num() int
	MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value
	UnmarshalRLPFrom(*fastrlp.Parser, *fastrlp.Value) error
}

// parseRound parses RLP-encoded bytes into round
func parseRound(v *fastrlp.Value) (*uint64, error) {
	roundBytes, err := v.Bytes()
	if err != nil {
		return nil, err
	}

	if len(roundBytes) > 8 {
		return nil, errRoundNumberOverflow
	}

	if len(roundBytes) == 0 {
		return nil, nil
	}

	round := binary.BigEndian.Uint64(roundBytes)

	return &round, nil
}

// toRoundBytes converts uint64 round to bytes
// Round begins with zero and it can be nil for backward compatibility.
// For that reason, Extra always has 8 bytes space for a round when the round has value.
func toRoundBytes(round uint64) []byte {
	roundBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(roundBytes, round)

	return roundBytes
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
	if i.ParentCommittedSeals == nil {
		vv.Set(ar.NewNullArray())
	} else {
		vv.Set(i.ParentCommittedSeals.MarshalRLPWith(ar))
	}

	if i.RoundNumber == nil {
		vv.Set(ar.NewNull())
	} else {
		vv.Set(ar.NewBytes(
			toRoundBytes(*i.RoundNumber),
		))
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

	// Round
	if len(elems) >= 5 {
		roundNumber, err := parseRound(elems[4])
		if err != nil {
			return err
		}

		i.RoundNumber = roundNumber
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

	// Round
	if len(elems) >= 5 {
		roundNumber, err := parseRound(elems[4])
		if err != nil {
			return err
		}

		i.RoundNumber = roundNumber
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

			// Round
			if len(oldValues) >= 5 {
				newArrayValue.Set(oldValues[4])
			}

			return nil
		},
	)
}

// packCommittedSealsAndRoundNumberIntoExtra updates only CommittedSeal field in Extra
func packCommittedSealsAndRoundNumberIntoExtra(
	extraBytes []byte,
	committedSeal Seals,
	roundNumber *uint64,
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
			} else {
				newArrayValue.Set(ar.NewNullArray())
			}

			if roundNumber == nil {
				newArrayValue.Set(ar.NewNull())
			} else {
				newArrayValue.Set(ar.NewBytes(
					toRoundBytes(*roundNumber),
				))
			}

			return nil
		},
	)
}
