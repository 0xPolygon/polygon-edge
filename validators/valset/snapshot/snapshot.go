package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
	"github.com/hashicorp/go-hclog"
)

const (
	loggerName = "snapshot_validator_set"
)

var (
	// Magic nonce number to vote on adding a new validator
	nonceAuthVote = types.Nonce{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

	// Magic nonce number to vote on removing a validator.
	nonceDropVote = types.Nonce{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

var (
	ErrInvalidNonce                 = errors.New("invalid nonce specified")
	ErrSnapshotNotFound             = errors.New("not found snapshot")
	ErrUnauthorizedProposer         = errors.New("unauthorized proposer")
	ErrIncorrectNonce               = errors.New("incorrect vote nonce")
	ErrAlreadyCandidate             = errors.New("already a candidate")
	ErrCandidateIsValidator         = errors.New("the candidate is already a validator")
	ErrCandidateNotExistInSet       = errors.New("cannot remove a validator if they're not in the snapshot")
	ErrAlreadyVoted                 = errors.New("already voted for this address")
	ErrMultipleVotesBySameValidator = errors.New("more than one proposal per validator per address found")
)

type SnapshotValidatorSet struct {
	// interface
	logger     hclog.Logger
	blockchain valset.HeaderGetter
	getSigner  valset.SignerGetter

	// configuration
	epochSize uint64

	// data
	store          *snapshotStore
	candidates     []*valset.Candidate
	candidatesLock sync.RWMutex
}

func NewSnapshotValidatorSet(
	logger hclog.Logger,
	blockchain valset.HeaderGetter,
	getSigner valset.SignerGetter,
	epochSize uint64,
	metadata *SnapshotMetadata,
	snapshots []*Snapshot,
) (valset.ValidatorSet, error) {
	set := &SnapshotValidatorSet{
		logger:         logger.Named(loggerName),
		store:          newSnapshotStore(metadata, snapshots),
		blockchain:     blockchain,
		getSigner:      getSigner,
		candidates:     make([]*valset.Candidate, 0),
		candidatesLock: sync.RWMutex{},
		epochSize:      epochSize,
	}

	if err := set.initialize(); err != nil {
		return nil, err
	}

	return set, nil
}

func (s *SnapshotValidatorSet) initialize() error {
	header := s.blockchain.Header()
	meta := s.GetSnapshotMetadata()

	if header.Number == 0 {
		// Genesis header needs to be set by hand, all the other
		// snapshots are set as part of processHeaders
		if err := s.addHeaderSnap(header); err != nil {
			return err
		}
	}

	// If the snapshot is not found, or the latest snapshot belongs to a previous epoch,
	// we need to start rebuilding the snapshot from the beginning of the current epoch
	// in order to have all the votes and validators correctly set in the snapshot,
	// since they reset every epoch.

	// Get epoch of latest header and saved metadata
	var (
		currentEpoch = header.Number / s.epochSize
		metaEpoch    = meta.LastBlock / s.epochSize

		snapshot = s.getSnapshot(header.Number)
	)

	if snapshot == nil || metaEpoch < currentEpoch {
		// Restore snapshot at the beginning of the current epoch by block header
		// if list doesn't have any snapshots to calculate snapshot for the next header
		s.logger.Info("snapshot was not found, restore snapshot at beginning of current epoch", "current epoch", currentEpoch)
		beginHeight := currentEpoch * s.epochSize
		beginHeader, ok := s.blockchain.GetHeaderByNumber(beginHeight)

		if !ok {
			return fmt.Errorf("header at %d not found", beginHeight)
		}

		if err := s.addHeaderSnap(beginHeader); err != nil {
			return err
		}

		s.store.updateLastBlock(beginHeight)

		meta = s.GetSnapshotMetadata()
	}

	// Process headers if we missed some blocks in the current epoch
	if header.Number > meta.LastBlock {
		s.logger.Info("syncing past snapshots", "from", meta.LastBlock, "to", header.Number)

		if err := s.ProcessHeadersInRange(meta.LastBlock+1, header.Number); err != nil {
			return err
		}
	}

	return nil
}

func (s *SnapshotValidatorSet) SourceType() valset.SourceType {
	return valset.Snapshot
}

func (s *SnapshotValidatorSet) GetSnapshotMetadata() *SnapshotMetadata {
	return &SnapshotMetadata{
		LastBlock: s.store.getLastBlock(),
	}
}

func (s *SnapshotValidatorSet) GetSnapshots() []*Snapshot {
	return s.store.list
}

func (s *SnapshotValidatorSet) GetValidators(height uint64, _ uint64) (validators.Validators, error) {
	var snapshotHeight uint64 = 0
	if int64(height) > 0 {
		snapshotHeight = height - 1
	}

	snapshot := s.getSnapshot(snapshotHeight)
	if snapshot == nil {
		return nil, ErrSnapshotNotFound
	}

	return snapshot.Set, nil
}

// Votes returns the votes in the snapshot at the specified height
func (s *SnapshotValidatorSet) Votes(height uint64) ([]*valset.Vote, error) {
	snapshot := s.getSnapshot(height)
	if snapshot == nil {
		return nil, ErrSnapshotNotFound
	}

	return snapshot.Votes, nil
}

// Candidates returns the current candidates
func (s *SnapshotValidatorSet) Candidates() []*valset.Candidate {
	return s.candidates
}

func (s *SnapshotValidatorSet) UpdateSet(newValidators validators.Validators, from uint64) error {
	snapshotHeight := from - 1

	snapshot := s.getSnapshot(snapshotHeight)
	if snapshot == nil {
		snapshot = &Snapshot{}
	}

	newSnapshot := snapshot.Copy()

	header, _ := s.blockchain.GetHeaderByNumber(from - 1)
	if header == nil {
		return fmt.Errorf("header at %d not found", from-1)
	}

	newSnapshot.Number = snapshotHeight
	newSnapshot.Hash = header.Hash.String()
	newSnapshot.Set = newValidators
	newSnapshot.Votes = []*valset.Vote{}

	if !newSnapshot.Equal(snapshot) {
		s.store.add(newSnapshot)
	}

	return nil
}

func (s *SnapshotValidatorSet) ModifyHeader(header *types.Header, proposer types.Address) error {
	snapshot := s.getSnapshot(header.Number)
	if snapshot == nil {
		return ErrSnapshotNotFound
	}

	if candidate := s.getNextCandidate(snapshot, proposer); candidate != nil {
		var err error

		header.Miner, err = validatorToMiner(candidate.Validator)
		if err != nil {
			return err
		}

		if candidate.Authorize {
			header.Nonce = nonceAuthVote
		} else {
			header.Nonce = nonceDropVote
		}
	}

	return nil
}

func (s *SnapshotValidatorSet) VerifyHeader(header *types.Header) error {
	// Check the nonce format.
	// The nonce field must have either an AUTH or DROP vote value.
	// Block nonce values are not taken into account when the Miner field is set to zeroes, indicating
	// no vote casting is taking place within a block
	if header.Nonce != nonceAuthVote && header.Nonce != nonceDropVote {
		return ErrInvalidNonce
	}

	return nil
}

// ProcessHeadersInRange is a helper function process headers in the given range
func (s *SnapshotValidatorSet) ProcessHeadersInRange(
	from, to uint64,
) error {
	for i := from; i <= to; i++ {
		if i == 0 {
			continue
		}

		header, ok := s.blockchain.GetHeaderByNumber(i)
		if !ok {
			return fmt.Errorf("header %d not found", i)
		}

		if err := s.ProcessHeader(header); err != nil {
			return err
		}
	}

	return nil
}

func (s *SnapshotValidatorSet) ProcessHeader(
	header *types.Header,
) error {
	signer, err := s.getSigner(header.Number)
	if err != nil {
		return err
	}

	if signer == nil {
		return fmt.Errorf("signer not found at %d", header.Number)
	}

	proposer, err := signer.EcrecoverFromHeader(header)
	if err != nil {
		return err
	}

	isValidator, err := s.isValidator(proposer, header.Number)
	if err != nil {
		return err
	}

	if !isValidator {
		return ErrUnauthorizedProposer
	}

	parentSnap := s.getSnapshot(header.Number - 1)
	if parentSnap == nil {
		return ErrSnapshotNotFound
	}

	snap := parentSnap.Copy()

	// Reset votes when new epoch
	if header.Number%s.epochSize == 0 {
		s.resetSnapshot(parentSnap, snap, header)
		s.removeLowerSnapshots(header.Number)
		s.store.updateLastBlock(header.Number)

		return nil
	}

	// no vote if miner field is not set
	if bytes.Equal(header.Miner, types.ZeroAddress[:]) {
		s.store.updateLastBlock(header.Number)

		return nil
	}

	// Process votes in the middle of epoch
	return s.processVote(header, signer, proposer, parentSnap, snap)
}

func (s *SnapshotValidatorSet) Propose(candidate validators.Validator, auth bool, proposer types.Address) error {
	s.candidatesLock.Lock()
	defer s.candidatesLock.Unlock()

	candidateAddr := candidate.Addr()

	for _, c := range s.candidates {
		if c.Validator.Addr() == candidateAddr {
			return ErrAlreadyCandidate
		}
	}

	snap := s.getLatestSnapshot()
	if snap == nil {
		return ErrSnapshotNotFound
	}

	included := snap.Set.Includes(candidateAddr)

	// safe checks
	if auth && included {
		return ErrCandidateIsValidator
	} else if !auth && !included {
		return ErrCandidateNotExistInSet
	}

	// check if we have already voted for this candidate
	count := snap.CountByVoterAndCandidate(proposer, candidate)
	if count == 1 {
		return ErrAlreadyVoted
	}

	return s.addCandidate(
		snap.Set,
		candidate,
		auth,
	)
}

// AddCandidate adds new candidate to candidate list
func (s *SnapshotValidatorSet) addCandidate(
	validators validators.Validators,
	candidate validators.Validator,
	authrorize bool,
) error {
	if authrorize {
		s.candidates = append(s.candidates, &valset.Candidate{
			Validator: candidate,
			Authorize: authrorize,
		})

		return nil
	}

	// get candidate validator information from set
	// because don't want user to specify data except for address
	// in case of removal
	validatorIndex := validators.Index(candidate.Addr())
	if validatorIndex == -1 {
		return ErrCandidateNotExistInSet
	}

	s.candidates = append(s.candidates, &valset.Candidate{
		Validator: validators.At(uint64(validatorIndex)),
		Authorize: authrorize,
	})

	return nil
}

// addHeaderSnap creates the initial snapshot, and adds it to the snapshot store
func (s *SnapshotValidatorSet) addHeaderSnap(header *types.Header) error {
	signer, err := s.getSigner(header.Number)
	if err != nil {
		return err
	}

	if signer == nil {
		return fmt.Errorf("signer not found %d", header.Number)
	}

	extra, err := signer.GetIBFTExtra(header)
	if err != nil {
		return err
	}

	// Create the first snapshot from the genesis
	s.store.add(&Snapshot{
		Hash:   header.Hash.String(),
		Number: header.Number,
		Votes:  []*valset.Vote{},
		Set:    extra.Validators,
	})

	return nil
}

func (s *SnapshotValidatorSet) getSnapshot(height uint64) *Snapshot {
	return s.store.find(height)
}

func (s *SnapshotValidatorSet) getLatestSnapshot() *Snapshot {
	return s.getSnapshot(s.store.lastNumber)
}

func (s *SnapshotValidatorSet) getNextCandidate(
	snap *Snapshot,
	proposer types.Address,
) *valset.Candidate {
	s.candidatesLock.Lock()
	defer s.candidatesLock.Unlock()

	// first, we need to remove any candidates that have already been
	// selected as validators
	s.cleanObsoleteCandidates(snap.Set)

	// now pick the first candidate that has not received a vote yet
	return s.pickOneCandidate(snap, proposer)
}

// cleanObsolateCandidates removes useless candidates from candidates field
// Unsafe against concurrent accesses
func (s *SnapshotValidatorSet) cleanObsoleteCandidates(set validators.Validators) {
	newCandidates := make([]*valset.Candidate, 0, len(s.candidates))

	for _, candidate := range s.candidates {
		// If Authorize is
		// true => Candidate needs to be in Set
		// false => Candidate needs not to be in Set
		// if the current situetion is not so, it's still a candidate
		if candidate.Authorize != set.Includes(candidate.Validator.Addr()) {
			newCandidates = append(newCandidates, candidate)
		}
	}

	s.candidates = newCandidates
}

// pickOneCandidate returns a propser candidate from candidates field
// Unsafe against concurrent accesses
func (s *SnapshotValidatorSet) pickOneCandidate(
	snap *Snapshot,
	proposer types.Address,
) *valset.Candidate {
	for _, c := range s.candidates {
		addr := c.Validator.Addr()

		count := snap.Count(func(v *valset.Vote) bool {
			return v.Candidate.Addr() == addr && v.Validator == proposer
		})

		if count == 0 {
			return c
		}
	}

	return nil
}

// isValidator is a helper function to returns a validator with the given address is
// a validator in the specified height
func (s *SnapshotValidatorSet) isValidator(
	address types.Address,
	height uint64,
) (bool, error) {
	// Check if the recovered proposer is part of the validator set
	vals, err := s.GetValidators(height, 0)
	if err != nil {
		return false, err
	}

	return vals.Includes(address), nil
}

// saveSnapshotIfChanged is a helper method to save snapshot updated by the given header
// only if the snapshot is updated from parent snapshot
func (s *SnapshotValidatorSet) saveSnapshotIfChanged(
	parentSnapshot, snapshot *Snapshot,
	header *types.Header,
) {
	if snapshot.Equal(parentSnapshot) {
		return
	}

	snapshot.Number = header.Number
	snapshot.Hash = header.Hash.String()

	s.store.add(snapshot)
}

// resetSnapshot is a helper method to save a snapshot that clears votes
func (s *SnapshotValidatorSet) resetSnapshot(
	parentSnapshot, snapshot *Snapshot,
	header *types.Header,
) {
	snapshot.Votes = nil

	s.saveSnapshotIfChanged(parentSnapshot, snapshot, header)
}

// removeLowerSnapshots is a helper function to removes old snapshots
func (s *SnapshotValidatorSet) removeLowerSnapshots(
	currentHeight uint64,
) {
	// remove in-memory snapshots from two epochs before this one
	lowerEpoch := int(currentHeight/s.epochSize) - 2
	if lowerEpoch > 0 {
		purgeBlock := uint64(lowerEpoch) * s.epochSize
		s.store.deleteLower(purgeBlock)
	}
}

// processVote processes a vote in header
func (s *SnapshotValidatorSet) processVote(
	header *types.Header,
	signer signer.Signer,
	proposer types.Address,
	parentSnapshot, snapshot *Snapshot,
) error {
	// the nonce selects the action
	authorize, err := isAuthorize(header.Nonce)
	if err != nil {
		return err
	}

	validatorType := signer.Type()

	// parse candidate validator set from header.Miner
	candidate, err := minerToValidator(validatorType, header.Miner)
	if err != nil {
		return err
	}

	// if candidate has been processed as expected, just update last block
	if !shouldProcessVote(snapshot.Set, candidate.Addr(), authorize) {
		s.store.updateLastBlock(header.Number)

		return nil
	}

	voteCount := snapshot.CountByVoterAndCandidate(proposer, candidate)
	if voteCount > 1 {
		// there can only be one vote per validator per address
		return ErrMultipleVotesBySameValidator
	}

	if voteCount == 0 {
		// cast the new vote since there is no one yet
		snapshot.AddVote(proposer, candidate, authorize)
	}

	// check the tally for the proposed validator
	totalVotes := snapshot.CountByCandidate(candidate)

	// If more than a half of all validators voted
	if totalVotes > snapshot.Set.Len()/2 {
		if err := addsOrDelsCandidate(
			snapshot.Set,
			candidate,
			authorize,
		); err != nil {
			return err
		}

		if !authorize {
			// remove any votes casted by the removed validator
			snapshot.RemoveVotesByVoter(candidate.Addr())
		}

		// remove all the votes that promoted this validator
		snapshot.RemoveVotesByCandidate(candidate)
	}

	s.saveSnapshotIfChanged(parentSnapshot, snapshot, header)
	s.store.updateLastBlock(header.Number)

	return nil
}

// validatorToMiner converts validator to bytes for miner field in header
func validatorToMiner(validator validators.Validator) ([]byte, error) {
	switch validator.(type) {
	case *validators.ECDSAValidator:
		// Return Address directly
		// to support backward compatibility
		return validator.Addr().Bytes(), nil
	case *validators.BLSValidator:
		return validator.Bytes(), nil
	default:
		return nil, validators.ErrInvalidValidatorType
	}
}

// minerToValidator converts bytes to validator for miner field in header
func minerToValidator(
	validatorType validators.ValidatorType,
	miner []byte,
) (validators.Validator, error) {
	validator, err := validators.NewValidatorFromType(validatorType)
	if err != nil {
		return nil, err
	}

	switch typedVal := validator.(type) {
	case *validators.ECDSAValidator:
		typedVal.Address = types.BytesToAddress(miner)
	case *validators.BLSValidator:
		if err := typedVal.SetFromBytes(miner); err != nil {
			return nil, err
		}
	default:
		// shouldn't reach here
		return nil, validators.ErrInvalidValidatorType
	}

	return validator, nil
}
