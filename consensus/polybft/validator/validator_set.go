package validator

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/fastrlp"
)

// ValidatorSet interface of the current validator set
type ValidatorSet interface {
	// Includes check if given address is among the current validator set
	Includes(address types.Address) bool

	// Len returns the size of the validator set
	Len() int

	// Accounts returns the list of the ValidatorMetadata
	Accounts() AccountSet

	// checks if submitted signers have reached quorum
	HasQuorum(signers map[types.Address]struct{}) bool
}

type validatorSet struct {
	// validators represents current list of validators
	validators AccountSet

	// votingPowerMap represents voting powers per validator address
	votingPowerMap map[types.Address]*big.Int

	// totalVotingPower is sum of active validator set
	totalVotingPower *big.Int

	// quorumSize is 2/3 super-majority of totalVotingPower
	quorumSize *big.Int

	// logger instance
	logger hclog.Logger
}

// NewValidatorSet creates a new validator set.
func NewValidatorSet(valz AccountSet, logger hclog.Logger) *validatorSet {
	totalVotingPower := valz.GetTotalVotingPower()
	quorumSize := getQuorumSize(totalVotingPower)

	votingPowerMap := make(map[types.Address]*big.Int, len(valz))
	for _, val := range valz {
		votingPowerMap[val.Address] = val.VotingPower
	}

	return &validatorSet{
		validators:       valz,
		votingPowerMap:   votingPowerMap,
		totalVotingPower: totalVotingPower,
		quorumSize:       quorumSize,
		logger:           logger.Named("validator_set"),
	}
}

// HasQuorum determines if there is quorum of enough signers reached,
// based on its voting power and quorum size
func (vs validatorSet) HasQuorum(signers map[types.Address]struct{}) bool {
	aggregateVotingPower := big.NewInt(0)

	for address := range signers {
		if votingPower := vs.votingPowerMap[address]; votingPower != nil {
			_ = aggregateVotingPower.Add(aggregateVotingPower, votingPower)
		}
	}

	hasQuorum := aggregateVotingPower.Cmp(vs.quorumSize) >= 0

	vs.logger.Debug("HasQuorum",
		"signers", len(signers),
		"signers voting power", aggregateVotingPower,
		"hasQuorum", hasQuorum)

	return hasQuorum
}

func (vs validatorSet) Accounts() AccountSet {
	return vs.validators
}

func (vs validatorSet) Includes(address types.Address) bool {
	return vs.validators.ContainsAddress(address)
}

func (vs validatorSet) Len() int {
	return vs.validators.Len()
}

func (vs validatorSet) TotalVotingPower() big.Int {
	return *vs.totalVotingPower
}

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

// getQuorumSize calculates quorum size as 2/3 super-majority of provided total voting power
func getQuorumSize(totalVotingPower *big.Int) *big.Int {
	quorum := new(big.Int)
	quorum.Mul(totalVotingPower, big.NewInt(2))

	return common.BigIntDivCeil(quorum, big.NewInt(3))
}
