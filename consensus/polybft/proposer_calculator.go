package polybft

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	bolt "go.etcd.io/bbolt"
)

var (
	// priorityWindowSizeFactor - is a constant that when multiplied with the
	// total voting power gives the maximum allowed distance between validator
	// priorities.
	priorityWindowSizeFactor = big.NewInt(2)
)

// PrioritizedValidator holds ValidatorMetadata together with priority
type PrioritizedValidator struct {
	Metadata         *validator.ValidatorMetadata
	ProposerPriority *big.Int
}

func (pv PrioritizedValidator) String() string {
	return fmt.Sprintf("[%v, voting power %v, priority %v]", pv.Metadata.Address.String(),
		pv.Metadata.VotingPower, pv.ProposerPriority)
}

// ProposerSnapshot represents snapshot of one proposer calculation
type ProposerSnapshot struct {
	Height     uint64
	Round      uint64
	Proposer   *PrioritizedValidator
	Validators []*PrioritizedValidator
}

// NewProposerSnapshotFromState create ProposerSnapshot from state if possible or from genesis block
func NewProposerSnapshotFromState(config *runtimeConfig, dbTx *bolt.Tx) (*ProposerSnapshot, error) {
	snapshot, err := config.State.ProposerSnapshotStore.getProposerSnapshot(dbTx)
	if err != nil {
		return nil, err
	}

	if snapshot == nil {
		// pick validator set from genesis block if snapshot is not saved in db
		genesisValidatorsSet, err := config.polybftBackend.GetValidatorsWithTx(0, nil, dbTx)
		if err != nil {
			return nil, err
		}

		snapshot = NewProposerSnapshot(1, genesisValidatorsSet)
	}

	return snapshot, nil
}

// NewProposerSnapshot creates ProposerSnapshot with height and validators with all priorities set to zero
func NewProposerSnapshot(height uint64, validators []*validator.ValidatorMetadata) *ProposerSnapshot {
	validatorsSnap := make([]*PrioritizedValidator, len(validators))

	for i, x := range validators {
		validatorsSnap[i] = &PrioritizedValidator{Metadata: x, ProposerPriority: big.NewInt(0)}
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

// GetTotalVotingPower returns total voting power from all the validators
func (pcs ProposerSnapshot) GetTotalVotingPower() *big.Int {
	totalVotingPower := new(big.Int)
	for _, v := range pcs.Validators {
		totalVotingPower.Add(totalVotingPower, v.Metadata.VotingPower)
	}

	return totalVotingPower
}

// Copy Returns copy of current ProposerSnapshot object
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
func NewProposerCalculator(config *runtimeConfig, logger hclog.Logger,
	dbTx *bolt.Tx) (*ProposerCalculator, error) {
	snap, err := NewProposerSnapshotFromState(config, dbTx)

	if err != nil {
		return nil, err
	}

	pc := &ProposerCalculator{
		snapshot: snap,
		config:   config,
		state:    config.State,
		logger:   logger,
	}

	// If the node was previously stopped, leaving the proposer calculator in an inconsistent state,
	// proposer calculator needs to be updated.
	blockNumber := config.blockchain.CurrentHeader().Number
	if pc.snapshot.Height <= blockNumber {
		if err = pc.update(blockNumber, dbTx); err != nil {
			return nil, err
		}
	}

	return pc, nil
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

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It will update priorities and save the updated snapshot to db
func (pc *ProposerCalculator) PostBlock(req *PostBlockRequest) error {
	return pc.update(req.FullBlock.Block.Number(), req.DBTx)
}

func (pc *ProposerCalculator) update(blockNumber uint64, dbTx *bolt.Tx) error {
	pc.logger.Debug("Update proposers snapshot started", "target block", blockNumber)

	from := pc.snapshot.Height

	// using a for loop if in some previous block, an error occurred while updating snapshot
	// so that we can recalculate it to have accurate priorities.
	// Note, this will change once we introduce component wide global transaction
	for height := from; height <= blockNumber; height++ {
		if err := pc.updatePerBlock(height, dbTx); err != nil {
			return err
		}

		pc.logger.Debug("Proposer snapshot has been updated",
			"block", height, "validators", pc.snapshot.Validators)
	}

	if err := pc.state.ProposerSnapshotStore.writeProposerSnapshot(pc.snapshot, dbTx); err != nil {
		return fmt.Errorf("cannot save proposers snapshot for block %d: %w", blockNumber, err)
	}

	pc.logger.Info("Proposer snapshot update has been finished",
		"target block", blockNumber+1, "validators", len(pc.snapshot.Validators))

	return nil
}

// Updates ProposerSnapshot to block block with number `blockNumber`
func (pc *ProposerCalculator) updatePerBlock(blockNumber uint64, dbTx *bolt.Tx) error {
	if pc.snapshot.Height != blockNumber {
		return fmt.Errorf("proposers snapshot update called for wrong block. block number=%d, snapshot block number=%d",
			blockNumber, pc.snapshot.Height)
	}

	_, extra, err := getBlockData(blockNumber, pc.config.blockchain)
	if err != nil {
		return fmt.Errorf("cannot get block header and extra while updating proposers snapshot %d: %w", blockNumber, err)
	}

	var newValidatorSet validator.AccountSet = nil

	if extra.Validators != nil && !extra.Validators.IsEmpty() {
		newValidatorSet, err = pc.config.polybftBackend.GetValidatorsWithTx(blockNumber, nil, dbTx)
		if err != nil {
			return fmt.Errorf("cannot get validators for block %d: %w", blockNumber, err)
		}
	}

	// if round = 0 then we need one iteration
	_, err = incrementProposerPriorityNTimes(pc.snapshot, extra.Checkpoint.BlockRound+1)
	if err != nil {
		return fmt.Errorf("failed to update proposers snapshot for block %d: %w", blockNumber, err)
	}

	// update to new validator set and center if needed
	if err = updateValidators(pc.snapshot, newValidatorSet); err != nil {
		return fmt.Errorf("cannot update validators: %w", err)
	}

	pc.snapshot.Height = blockNumber + 1 // snapshot (validator priorities) is prepared for the next block
	pc.snapshot.Round = 0
	pc.snapshot.Proposer = nil

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

func updateValidators(snapshot *ProposerSnapshot, newValidatorSet validator.AccountSet) error {
	if newValidatorSet.Len() == 0 {
		return nil
	}

	snapshotValidators := snapshot.toMap()
	newValidators := make([]*PrioritizedValidator, len(newValidatorSet))

	// compute total voting power of removed validators and current validator
	removedValidatorsVotingPower := new(big.Int)
	newValidatorsVotingPower := new(big.Int)

	for address, val := range snapshotValidators {
		if !newValidatorSet.ContainsNodeID(address.String()) {
			removedValidatorsVotingPower.Add(removedValidatorsVotingPower, val.Metadata.VotingPower)
		}
	}

	for _, v := range newValidatorSet {
		newValidatorsVotingPower.Add(newValidatorsVotingPower, v.VotingPower)
	}

	tvpAfterUpdatesBeforeRemovals := new(big.Int).Add(newValidatorsVotingPower, removedValidatorsVotingPower)

	for i, newValidator := range newValidatorSet {
		if val, exists := snapshotValidators[newValidator.Address]; exists {
			// old validators have the same priority
			newValidators[i] = &PrioritizedValidator{
				Metadata:         newValidator,
				ProposerPriority: val.ProposerPriority,
			}
		} else {
			// added validator has priority = -C*totalVotingPowerBeforeRemoval (with C ~= 1.125)
			coefficient := new(big.Int).Div(tvpAfterUpdatesBeforeRemovals, big.NewInt(8))
			newValidators[i] = &PrioritizedValidator{
				Metadata:         newValidator,
				ProposerPriority: new(big.Int).Neg(tvpAfterUpdatesBeforeRemovals.Add(tvpAfterUpdatesBeforeRemovals, coefficient)),
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

func incrementProposerPriority(snapshot *ProposerSnapshot, totalVotingPower *big.Int) (*PrioritizedValidator, error) {
	for _, val := range snapshot.Validators {
		// Check for overflow for sum.
		newPrio := new(big.Int).Add(val.ProposerPriority, val.Metadata.VotingPower)
		val.ProposerPriority = newPrio
	}
	// Decrement the validator with most ProposerPriority.
	mostest, err := getValWithMostPriority(snapshot)
	if err != nil {
		return nil, fmt.Errorf("cannot get validator with most priority: %w", err)
	}

	mostest.ProposerPriority.Sub(mostest.ProposerPriority, totalVotingPower)

	return mostest, nil
}

func updateWithChangeSet(snapshot *ProposerSnapshot, totalVotingPower *big.Int) error {
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
		val.ProposerPriority.Sub(val.ProposerPriority, avgProposerPriority)
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

func computeAvgProposerPriority(snapshot *ProposerSnapshot) (*big.Int, error) {
	if len(snapshot.Validators) == 0 {
		return nil, fmt.Errorf("validator set cannot be nul or empty")
	}

	validatorsCount := big.NewInt(int64(len(snapshot.Validators)))
	sum := new(big.Int)

	for _, val := range snapshot.Validators {
		sum = sum.Add(sum, val.ProposerPriority)
	}

	return sum.Div(sum, validatorsCount), nil
}

// rescalePriorities rescales the priorities such that the distance between the
// maximum and minimum is smaller than `diffMax`.
func rescalePriorities(snapshot *ProposerSnapshot, totalVotingPower *big.Int) error {
	if len(snapshot.Validators) == 0 {
		return fmt.Errorf("validator set cannot be nul or empty")
	}

	// Cap the difference between priorities to be proportional to 2*totalPower by
	// re-normalizing priorities, i.e., rescale all priorities by multiplying with:
	// 2*totalVotingPower/(maxPriority - minPriority)
	diffMax := new(big.Int).Mul(priorityWindowSizeFactor, totalVotingPower)

	// Calculating ceil(diff/diffMax):
	// Re-normalization is performed by dividing by an integer for simplicity.
	// NOTE: This may make debugging priority issues easier as well.
	diff := computeMaxMinPriorityDiff(snapshot.Validators)
	ratio := common.BigIntDivCeil(diff, diffMax)

	if diff.Cmp(diffMax) > 0 {
		for _, val := range snapshot.Validators {
			val.ProposerPriority.Quo(val.ProposerPriority, ratio)
		}
	}

	return nil
}

// computeMaxMinPriorityDiff computes the difference between the max and min ProposerPriority of that set.
func computeMaxMinPriorityDiff(validators []*PrioritizedValidator) *big.Int {
	min, max := validators[0].ProposerPriority, validators[0].ProposerPriority

	for _, v := range validators[1:] {
		if v.ProposerPriority.Cmp(min) < 0 {
			min = v.ProposerPriority
		}

		if v.ProposerPriority.Cmp(max) > 0 {
			max = v.ProposerPriority
		}
	}

	diff := new(big.Int).Sub(max, min)

	if diff.Cmp(big.NewInt(0)) < 0 {
		return diff.Neg(diff)
	}

	return diff
}

// isBetterProposer compares provided PrioritizedValidator instances
// and chooses either one with higher ProposerPriority or the one with the smaller address (compared lexicographically).
func isBetterProposer(a, b *PrioritizedValidator) bool {
	if b == nil || a.ProposerPriority.Cmp(b.ProposerPriority) > 0 {
		return true
	} else if a.ProposerPriority == b.ProposerPriority {
		return bytes.Compare(a.Metadata.Address.Bytes(), b.Metadata.Address.Bytes()) <= 0
	}

	return false
}
