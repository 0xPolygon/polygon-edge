package ibft

import (
	"strings"

	"github.com/0xPolygon/minimal/types"
)

type Validator interface {
	// Address returns address
	Address() types.Address

	// String representation of Validator
	String() string
}

// ----------------------------------------------------------------------------

type Validators []Validator

func (slice Validators) Len() int {
	return len(slice)
}

func (slice Validators) Less(i, j int) bool {
	return strings.Compare(slice[i].String(), slice[j].String()) < 0
}

func (slice Validators) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

// ----------------------------------------------------------------------------

type ValidatorSet interface {
	// Calculate the proposer
	CalcProposer(lastProposer types.Address, round uint64)
	// Return the validator size
	Size() int
	// Return the validator array
	List() []Validator
	// Get validator by index
	GetByIndex(i uint64) Validator
	// Get validator by given address
	GetByAddress(addr types.Address) (int, Validator)
	// Get current proposer
	GetProposer() Validator
	// Check whether the validator with given address is a proposer
	IsProposer(address types.Address) bool
	// Add validator
	AddValidator(address types.Address) bool
	// Remove validator
	RemoveValidator(address types.Address) bool
	// Copy validator set
	Copy() ValidatorSet
	// Get the maximum number of faulty nodes
	F() int
	// Get proposer policy
	Policy() ProposerPolicy
}

// ----------------------------------------------------------------------------

type ProposalSelector func(ValidatorSet, types.Address, uint64) Validator
