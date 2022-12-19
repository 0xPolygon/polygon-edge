package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	// maxTotalVotingPower - the maximum allowed total voting power.
	// It needs to be sufficiently small to, in all cases:
	// 1. prevent clipping in incrementProposerPriority()
	// 2. let (diff+diffMax-1) not overflow in IncrementProposerPriority()
	// (Proof of 1 is tricky, left to the reader).
	// It could be higher, but this is sufficiently large for our purposes,
	// and leaves room for defensive purposes.
	maxTotalVotingPower = int64(math.MaxInt64) / 8

	// priorityWindowSizeFactor - is a constant that when multiplied with the
	// total voting power gives the maximum allowed distance between validator
	// priorities.
	priorityWindowSizeFactor = 2
)

var (
	// errInvalidTotalVotingPower is returned if the total voting power is zero
	errInvalidTotalVotingPower = errors.New(
		"invalid voting power configuration provided: total voting power must be greater than 0")
)

// Holds ValidatorMetadata together with priority
type PrioritizedValidator struct {
	Metadata         *ValidatorMetadata
	ProposerPriority int64
}

// ProposerSnapshot represents snapshot of one proposer calculation
type ProposerSnapshot struct {
	Height     uint64
	Round      uint64
	Proposer   *PrioritizedValidator
	Validators []*PrioritizedValidator
	// logger     hclog.Logger
}

// NewProposerSnapshotFromState create ProposerSnapshot from state if possible or from genesis block
func NewProposerSnapshotFromState(config *runtimeConfig) (*ProposerSnapshot, error) {
	snapshot, err := config.State.getProposerSnapshot()
	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		// pick validator set from genesis block if snapshot is not saved in db
		genesisValidatorsSet, err := config.polybftBackend.GetValidators(0, nil)
		if err != nil {
			return nil, err
		}

		snapshot = NewProposerSnapshot(1, genesisValidatorsSet)
	}

	return snapshot, nil
}

// NewProposerSnapshot creates ProposerSnapshot with height and validators with all priorities set to zero
func NewProposerSnapshot(height uint64, validators []*ValidatorMetadata) *ProposerSnapshot {
	validatorsSnap := make([]*PrioritizedValidator, len(validators))

	for i, x := range validators {
		validatorsSnap[i] = &PrioritizedValidator{Metadata: x, ProposerPriority: int64(0)}
	}

	return &ProposerSnapshot{
		Round:      0,
		Proposer:   nil,
		Height:     height,
		Validators: validatorsSnap,
	}
}

// CalcProposer calculates next proposer
func (pcs *ProposerSnapshot) CalcProposer(round, height uint64) (types.Address, error) {
	if height != pcs.Height {
		return types.ZeroAddress, fmt.Errorf("invalid height - expected %d, got %d", pcs.Height, height)
	}

	// optimization -> return current proposer if already calculated for this round
	if pcs.Round == round && pcs.Proposer != nil {
		return pcs.Proposer.Metadata.Address, nil
	}

	// do not change priorities on original snapshot while executing CalcProposer
	// if round = 0 then we need one iteration
	proposer, err := incrementProposerPriorityNTimes(pcs.Copy(), round+1)
	if err != nil {
		return types.ZeroAddress, err
	}

	pcs.Proposer = proposer
	pcs.Round = round

	return proposer.Metadata.Address, nil
}

// GetLatestProposer returns latest calculated proposer if any
func (pcs *ProposerSnapshot) GetLatestProposer(round, height uint64) (types.Address, error) {
	// round must be same as saved one and proposer must exist
	if pcs == nil || pcs.Proposer == nil || pcs.Round != round || pcs.Height != height {
		return types.ZeroAddress,
			fmt.Errorf("get latest proposer not found - height: %d, round: %d, pc height: %d, pc round: %d",
				height, round, pcs.Height, pcs.Round)
	}

	return pcs.Proposer.Metadata.Address, nil
}

// Gets total voting power from all the validators
func (pcs ProposerSnapshot) GetTotalVotingPower() int64 {
	totalVotingPower := int64(0)

	for _, v := range pcs.Validators {
		totalVotingPower = safeAddClip(totalVotingPower, int64(v.Metadata.VotingPower))
	}

	return totalVotingPower
}

// Returns copy of current ProposerSnapshot object
func (pcs *ProposerSnapshot) Copy() *ProposerSnapshot {
	var proposer *PrioritizedValidator

	valCopy := make([]*PrioritizedValidator, len(pcs.Validators))

	for i, val := range pcs.Validators {
		valCopy[i] = &PrioritizedValidator{Metadata: val.Metadata.Copy(), ProposerPriority: val.ProposerPriority}

		if pcs.Proposer != nil && pcs.Proposer.Metadata.Address == val.Metadata.Address {
			proposer = valCopy[i]
		}
	}

	return &ProposerSnapshot{
		Validators: valCopy,
		Height:     pcs.Height,
		Round:      pcs.Round,
		Proposer:   proposer,
	}
}

func (pcs *ProposerSnapshot) toMap() map[types.Address]*PrioritizedValidator {
	validatorMap := make(map[types.Address]*PrioritizedValidator)
	for _, v := range pcs.Validators {
		validatorMap[v.Metadata.Address] = v
	}

	return validatorMap
}

type ProposerCalculator struct {
	// current snapshot
	snapshot *ProposerSnapshot

	// runtime configuration
	config *runtimeConfig

	// state to save snapshot
	state *State

	// logger instance
	logger hclog.Logger
}

// NewProposerCalculator creates a new proposer calculator object
func NewProposerCalculator(config *runtimeConfig, logger hclog.Logger) (*ProposerCalculator, error) {
	snap, err := NewProposerSnapshotFromState(config)

	if err != nil {
		return nil, err
	}

	return &ProposerCalculator{
		snapshot: snap,
		config:   config,
		state:    config.State,
		logger:   logger,
	}, nil
}

// NewProposerCalculator creates a new proposer calculator object
func NewProposerCalculatorFromSnapshot(pcs *ProposerSnapshot, config *runtimeConfig,
	logger hclog.Logger) *ProposerCalculator {
	return &ProposerCalculator{
		snapshot: pcs.Copy(),
		config:   config,
		state:    config.State,
		logger:   logger,
	}
}

// Get copy of the proposers' snapshot
func (pc *ProposerCalculator) GetSnapshot() (*ProposerSnapshot, bool) {
	if pc.snapshot == nil {
		return nil, false
	}

	return pc.snapshot.Copy(), true
}

func (pc *ProposerCalculator) Update(blockNumber uint64) error {
	const saveEveryNIterations = 5

	pc.logger.Info("Update proposal snapshot started", "block", blockNumber)

	from := pc.snapshot.Height

	for height := from; height <= blockNumber; height++ {
		if err := pc.updatePerBlock(height); err != nil {
			return err
		}

		// write snapshot every saveEveryNIterations iterations
		// this way, we prevent data loss on long calculations
		if (height-from+1)%saveEveryNIterations == 0 {
			if err := pc.state.writeProposerSnapshot(pc.snapshot); err != nil {
				return fmt.Errorf("cannot save proposer calculator snapshot for block %d: %w", height, err)
			}
		}
	}

	// write snapshot if not already written
	if (blockNumber-from+1)%saveEveryNIterations != 0 {
		if err := pc.state.writeProposerSnapshot(pc.snapshot); err != nil {
			return fmt.Errorf("cannot save proposer calculator snapshot for block %d: %w", blockNumber, err)
		}
	}

	// requires Lock
	pc.logger.Info("Update proposal snapshot finished", "block", blockNumber)

	return nil
}

// Updates ProposerSnapshot to block block with number `blockNumber`
func (pc *ProposerCalculator) updatePerBlock(blockNumber uint64) error {
	if pc.snapshot.Height != blockNumber {
		return fmt.Errorf("proposer calculator update wrong block=%d, height = %d", blockNumber, pc.snapshot.Height)
	}

	currentHeader, found := pc.config.blockchain.GetHeaderByNumber(blockNumber)
	if !found {
		return fmt.Errorf("cannot get header by number: %d", blockNumber)
	}

	extra, err := GetIbftExtra(currentHeader.ExtraData)
	if err != nil {
		return fmt.Errorf("cannot get ibft extra for block while updating proposer snapshot %d: %w", blockNumber, err)
	}

	var newValidatorSet AccountSet = nil

	if extra.Validators != nil && !extra.Validators.IsEmpty() {
		newValidatorSet, err = pc.config.polybftBackend.GetValidators(blockNumber, nil)
		if err != nil {
			return fmt.Errorf("cannot get ibft extra for block %d: %w", blockNumber, err)
		}
	}

	// if round = 0 then we need one iteration
	_, err = incrementProposerPriorityNTimes(pc.snapshot, extra.Checkpoint.BlockRound+1)
	if err != nil {
		return fmt.Errorf("failed to update calculator for block %d: %w", blockNumber, err)
	}

	// update to new validator set and center if needed
	updateValidators(pc.snapshot, newValidatorSet)

	pc.snapshot.Height = blockNumber + 1 // snapshot (validator priorities) is prepared for the next block
	pc.snapshot.Round = 0
	pc.snapshot.Proposer = nil

	pc.logger.Info("proposer calculator update has been finished", "height", blockNumber+1,
		"len", len(pc.snapshot.Validators))

	return nil
}

// algorithm functions receive snapshot and do appropriate calculations and changes
func incrementProposerPriorityNTimes(snapshot *ProposerSnapshot, times uint64) (*PrioritizedValidator, error) {
	if len(snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validator set cannot be nul or empty")
	}

	if times <= 0 {
		return nil, fmt.Errorf("cannot call IncrementProposerPriority with non-positive times")
	}

	var (
		proposer *PrioritizedValidator
		err      error
		tvp      = snapshot.GetTotalVotingPower()
	)

	if err := updateWithChangeSet(snapshot, tvp); err != nil {
		return nil, err
	}

	for i := uint64(0); i < times; i++ {
		if proposer, err = incrementProposerPriority(snapshot, tvp); err != nil {
			return nil, fmt.Errorf("cannot increment proposer priority: %w", err)
		}
	}

	snapshot.Proposer = proposer
	snapshot.Round = times - 1

	return proposer, nil
}

func updateValidators(snapshot *ProposerSnapshot, newValidatorSet AccountSet) error {
	if newValidatorSet.Len() == 0 {
		return nil
	}

	snapshotValidators := snapshot.toMap()
	newValidators := make([]*PrioritizedValidator, len(newValidatorSet))

	// compute total voting power of removed validators and current validator
	removedValidatorsVotingPower := uint64(0)
	newValidatorsVotingPower := uint64(0)

	for address, val := range snapshotValidators {
		if !newValidatorSet.ContainsNodeID(address.String()) {
			removedValidatorsVotingPower += val.Metadata.VotingPower
		}
	}

	for _, v := range newValidatorSet {
		newValidatorsVotingPower += v.VotingPower
	}

	if newValidatorsVotingPower > uint64(maxTotalVotingPower) {
		return fmt.Errorf(
			"total voting power cannot be guarded to not exceed %v; got: %v",
			maxTotalVotingPower,
			newValidatorsVotingPower,
		)
	}

	tvpAfterUpdatesBeforeRemovals := newValidatorsVotingPower + removedValidatorsVotingPower

	for i, newValidator := range newValidatorSet {
		if val, exists := snapshotValidators[newValidator.Address]; exists {
			// old validators have the same priority
			newValidators[i] = &PrioritizedValidator{
				Metadata:         newValidator,
				ProposerPriority: val.ProposerPriority,
			}
		} else {
			// added validator has priority = -C*totalVotingPowerBeforeRemoval (with C ~= 1.125)
			newValidators[i] = &PrioritizedValidator{
				Metadata:         newValidator,
				ProposerPriority: int64(-(tvpAfterUpdatesBeforeRemovals + (tvpAfterUpdatesBeforeRemovals >> 3))),
			}
		}
	}

	snapshot.Validators = newValidators

	// after validator set changes, center values around 0 and scale
	if err := updateWithChangeSet(snapshot, snapshot.GetTotalVotingPower()); err != nil {
		return fmt.Errorf("cannot update validator changeset: %w", err)
	}

	return nil
}

func incrementProposerPriority(snapshot *ProposerSnapshot, totalVotingPower int64) (*PrioritizedValidator, error) {
	for _, val := range snapshot.Validators {
		// Check for overflow for sum.
		newPrio := safeAddClip(val.ProposerPriority, int64(val.Metadata.VotingPower))
		val.ProposerPriority = newPrio
	}
	// Decrement the validator with most ProposerPriority.
	mostest, err := getValWithMostPriority(snapshot)
	if err != nil {
		return nil, fmt.Errorf("cannot get validator with most priority: %w", err)
	}

	mostest.ProposerPriority = safeSubClip(mostest.ProposerPriority, totalVotingPower)

	return mostest, nil
}

func updateWithChangeSet(snapshot *ProposerSnapshot, totalVotingPower int64) error {
	if err := rescalePriorities(snapshot, totalVotingPower); err != nil {
		return fmt.Errorf("cannot rescale priorities: %w", err)
	}

	if err := shiftByAvgProposerPriority(snapshot); err != nil {
		return fmt.Errorf("cannot shift proposer priorities: %w", err)
	}

	return nil
}

func shiftByAvgProposerPriority(snapshot *ProposerSnapshot) error {
	avgProposerPriority, err := computeAvgProposerPriority(snapshot)
	if err != nil {
		return fmt.Errorf("cannot compute proposer priority: %w", err)
	}

	for _, val := range snapshot.Validators {
		val.ProposerPriority = safeSubClip(val.ProposerPriority, avgProposerPriority)
	}

	return nil
}

func getValWithMostPriority(snapshot *ProposerSnapshot) (result *PrioritizedValidator, err error) {
	if len(snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validators cannot be nil or empty")
	}

	for _, curr := range snapshot.Validators {
		// pick curr as result if it has greater priority
		// or if it has same priority but "smaller" address
		if isBetterProposer(curr, result) {
			result = curr
		}
	}

	return result, nil
}

func computeAvgProposerPriority(snapshot *ProposerSnapshot) (int64, error) {
	if len(snapshot.Validators) == 0 {
		return 0, fmt.Errorf("validator set cannot be nul or empty")
	}

	n := int64(len(snapshot.Validators))
	sum := big.NewInt(0)

	for _, val := range snapshot.Validators {
		sum.Add(sum, big.NewInt(val.ProposerPriority))
	}

	avg := sum.Div(sum, big.NewInt(n))
	if avg.IsInt64() {
		return avg.Int64(), nil
	}

	return 0, fmt.Errorf("cannot represent avg ProposerPriority as an int64 %v", avg)
}

// rescalePriorities rescales the priorities such that the distance between the
// maximum and minimum is smaller than `diffMax`.
func rescalePriorities(snapshot *ProposerSnapshot, totalVotingPower int64) error {
	if len(snapshot.Validators) == 0 {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	// 2*totalVotingPower/(maxPriority - minPriority)
	diffMax := priorityWindowSizeFactor * totalVotingPower

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(snapshot.Validators)
	ratio := (diff + diffMax - 1) / diffMax

	if diff > diffMax {
		for _, val := range snapshot.Validators {
			val.ProposerPriority /= ratio
		}
	}

	return nil
}

// computeMaxMinPriorityDiff computes the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(validators []*PrioritizedValidator) int64 {
	max := int64(math.MinInt64)
	min := int64(math.MaxInt64)

	for _, v := range validators {
		if v.ProposerPriority < min {
			min = v.ProposerPriority
		}

		if v.ProposerPriority > max {
			max = v.ProposerPriority
		}
	}

	diff := max - min

	if diff < 0 {
		return -diff
	}

	return diff
}

func isBetterProposer(a, b *PrioritizedValidator) bool {
	if b == nil || a.ProposerPriority > b.ProposerPriority {
		return true
	} else if a.ProposerPriority == b.ProposerPriority {
		return bytes.Compare(a.Metadata.Address.Bytes(), b.Metadata.Address.Bytes()) <= 0
	} else {
		return false
	}
}
