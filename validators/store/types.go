package store

import (
	"encoding/json"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
)

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

// ValidatorStore is an interface that ValidatorStore needs to implement
type ValidatorStore interface {
	SourceType() SourceType
}

// HeaderGetter is an interface in order each ValidatorStore gets latest header and header by number
type HeaderGetter interface {
	Header() *types.Header
	GetHeaderByNumber(uint64) (*types.Header, bool)
}

// Vote defines the vote structure
type Vote struct {
	Validator types.Address        // Voter
	Candidate validators.Validator // Candidate
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
	return &Vote{
		Validator: v.Validator,
		Candidate: v.Candidate.Copy(),
		Authorize: v.Authorize,
	}
}

// UnmarshalJSON is JSON unmarshaler
func (v *Vote) UnmarshalJSON(data []byte) error {
	rawVote := struct {
		Validator types.Address // Voter
		Authorize bool          // Add or Remove

		Address   *types.Address  // Field in legacy format
		Candidate json.RawMessage // New field in new format
	}{}

	var err error

	if err = json.Unmarshal(data, &rawVote); err != nil {
		return err
	}

	v.Validator = rawVote.Validator
	v.Authorize = rawVote.Authorize

	// new format
	if rawVote.Candidate != nil {
		return json.Unmarshal(rawVote.Candidate, v.Candidate)
	}

	// legacy format
	if rawVote.Address != nil {
		ecdsaCandidate, ok := v.Candidate.(*validators.ECDSAValidator)
		if !ok {
			return fmt.Errorf("expects ECDSAValidator but got %s", v.Candidate.Type())
		}

		ecdsaCandidate.Address = *rawVote.Address
	}

	return nil
}

type Candidate struct {
	Validator validators.Validator
	Authorize bool
}
