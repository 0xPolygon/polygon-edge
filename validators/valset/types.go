package valset

import (
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

type SignerGetter func(uint64) (signer.Signer, error)

type ValidatorTypeGetter func(uint64) (validators.ValidatorType, error)

// Define the type of the validator set
type SourceType string

const (
	// For validators saved in-memory
	Snapshot SourceType = "Snapshot"

	// For validators managed in contract
	Contract SourceType = "Contract"
)

// String is a helper method for casting a SourceType to a string representation
func (t SourceType) String() string {
	return string(t)
}

type ValidatorSet interface {
	SourceType() SourceType
	GetValidators(uint64) (validators.Validators, error)
}

type HeaderModifier interface {
	ModifyHeader(*types.Header, types.Address) error
	VerifyHeader(*types.Header) error
}

type HeaderProcessor interface {
	ProcessHeader(*types.Header) error
}

type Updatable interface {
	UpdateSet(validators.Validators, uint64) error
}

type Votable interface {
	Votes(uint64) ([]*Vote, error)
	Candidates() []*Candidate
	Propose(validators.Validator, bool, types.Address) error
}

type HeaderGetter interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
}

// Vote defines the vote structure
type Vote struct {
	Validator types.Address        // Voter
	Candidate validators.Validator // Candidate of Validator
	Authorize bool                 // Add or Remove
}

// Equal checks if two votes are equal
func (v *Vote) Equal(vv *Vote) bool {
	if v.Validator != vv.Validator {
		return false
	}

	if !v.Candidate.Equal(vv.Candidate) {
		return false
	}

	if v.Authorize != vv.Authorize {
		return false
	}

	return true
}

// Copy makes a copy of the vote, and returns it
func (v *Vote) Copy() *Vote {
	vv := new(Vote)
	*vv = *v

	return vv
}

type Candidate struct {
	Validator validators.Validator
	Authorize bool
}
