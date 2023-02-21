package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
	"github.com/hashicorp/go-hclog"
)

const (
	loggerName      = "snapshot_validator_set"
	preservedEpochs = 2
)

// SignerInterface is an interface of the Signer SnapshotValidatorStore calls
type SignerInterface interface {
	Type() validators.ValidatorType
	EcrecoverFromHeader(*types.Header) (types.Address, error)
	GetValidators(*types.Header) (validators.Validators, error)
}

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

type SnapshotValidatorStore struct {
	// interface
	logger     hclog.Logger
	blockchain store.HeaderGetter
	getSigner  func(uint64) (SignerInterface, error)

	// configuration
	epochSize uint64

	// data
	store          *snapshotStore
	candidates     []*store.Candidate
	candidatesLock sync.RWMutex
}

// NewSnapshotValidatorStore creates and initializes *SnapshotValidatorStore
func NewSnapshotValidatorStore(
	logger hclog.Logger,
	blockchain store.HeaderGetter,
	getSigner func(uint64) (SignerInterface, error),
	epochSize uint64,
	metadata *SnapshotMetadata,
	snapshots []*Snapshot,
) (*SnapshotValidatorStore, error) {
	set := &SnapshotValidatorStore{
		logger:         logger.Named(loggerName),
		store:          newSnapshotStore(metadata, snapshots),
		blockchain:     blockchain,
		getSigner:      getSigner,
		candidates:     make([]*store.Candidate, 0),
		candidatesLock: sync.RWMutex{},
		epochSize:      epochSize,
	}

	if err := set.initialize(); err != nil {
		return nil, err
	}

	return set, nil
}

// initialize setup the snapshots to catch up latest header in blockchain
func (s *SnapshotValidatorStore) initialize() error {
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

// SourceType returns validator store type
func (s *SnapshotValidatorStore) SourceType() store.SourceType {
	return store.Snapshot
}

// GetSnapshotMetadata returns metadata
func (s *SnapshotValidatorStore) GetSnapshotMetadata() *SnapshotMetadata {
	return &SnapshotMetadata{
		LastBlock: s.store.getLastBlock(),
	}
}

// GetSnapshots returns all Snapshots
func (s *SnapshotValidatorStore) GetSnapshots() []*Snapshot {
	return s.store.list
}

// Candidates returns the current candidates
func (s *SnapshotValidatorStore) Candidates() []*store.Candidate {
	return s.candidates
}

// GetValidators returns the validator set in the Snapshot for the given height
func (s *SnapshotValidatorStore) GetValidatorsByHeight(height uint64) (validators.Validators, error) {
	snapshot := s.getSnapshot(height)
	if snapshot == nil {
		return nil, ErrSnapshotNotFound
	}

	return snapshot.Set, nil
}

// Votes returns the votes in the snapshot at the specified height
func (s *SnapshotValidatorStore) Votes(height uint64) ([]*store.Vote, error) {
	snapshot := s.getSnapshot(height)
	if snapshot == nil {
		return nil, ErrSnapshotNotFound
	}

	return snapshot.Votes, nil
}

// UpdateValidatorSet resets Snapshot with given validators at specified height
func (s *SnapshotValidatorStore) UpdateValidatorSet(
	// new validators to be overwritten
	newValidators validators.Validators,
	// the height from which new validators are used
	fromHeight uint64,
) error {
	snapshotHeight := fromHeight - 1

	header, ok := s.blockchain.GetHeaderByNumber(snapshotHeight)
	if !ok {
		return fmt.Errorf("header at %d not found", snapshotHeight)
	}

	s.store.putByNumber(&Snapshot{
		Number: header.Number,
		Hash:   header.Hash.String(),
		// reset validators & votes
		Set:   newValidators,
		Votes: []*store.Vote{},
	})

	return nil
}

// ModifyHeader updates Header to vote
func (s *SnapshotValidatorStore) ModifyHeader(header *types.Header, proposer types.Address) error {
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

// VerifyHeader verifies the fields of Header which are modified in ModifyHeader
func (s *SnapshotValidatorStore) VerifyHeader(header *types.Header) error {
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
func (s *SnapshotValidatorStore) ProcessHeadersInRange(
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

// ProcessHeader processes the header and updates snapshots
func (s *SnapshotValidatorStore) ProcessHeader(
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

	parentSnap := s.getSnapshot(header.Number - 1)
	if parentSnap == nil {
		return ErrSnapshotNotFound
	}

	if !parentSnap.Set.Includes(proposer) {
		return ErrUnauthorizedProposer
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
	if err := processVote(snap, header, signer.Type(), proposer); err != nil {
		return err
	}

	s.store.updateLastBlock(header.Number)
	s.saveSnapshotIfChanged(parentSnap, snap, header)

	return nil
}

// Propose adds new candidate for vote
func (s *SnapshotValidatorStore) Propose(candidate validators.Validator, auth bool, proposer types.Address) error {
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
// unsafe against concurrent access
func (s *SnapshotValidatorStore) addCandidate(
	validators validators.Validators,
	candidate validators.Validator,
	authrorize bool,
) error {
	if authrorize {
		s.candidates = append(s.candidates, &store.Candidate{
			Validator: candidate,
			Authorize: authrorize,
		})

		return nil
	}

	// Get candidate validator information from set
	// because don't want user to specify data except for address
	// in case of removal
	validatorIndex := validators.Index(candidate.Addr())
	if validatorIndex == -1 {
		return ErrCandidateNotExistInSet
	}

	s.candidates = append(s.candidates, &store.Candidate{
		Validator: validators.At(uint64(validatorIndex)),
		Authorize: authrorize,
	})

	return nil
}

// addHeaderSnap creates the initial snapshot, and adds it to the snapshot store
func (s *SnapshotValidatorStore) addHeaderSnap(header *types.Header) error {
	signer, err := s.getSigner(header.Number)
	if err != nil {
		return err
	}

	if signer == nil {
		return fmt.Errorf("signer not found %d", header.Number)
	}

	validators, err := signer.GetValidators(header)
	if err != nil {
		return err
	}

	// Create the first snapshot from the genesis
	s.store.add(&Snapshot{
		Hash:   header.Hash.String(),
		Number: header.Number,
		Votes:  []*store.Vote{},
		Set:    validators,
	})

	return nil
}

// getSnapshot returns a snapshot for specified height
func (s *SnapshotValidatorStore) getSnapshot(height uint64) *Snapshot {
	return s.store.find(height)
}

// getLatestSnapshot returns a snapshot for latest height
func (s *SnapshotValidatorStore) getLatestSnapshot() *Snapshot {
	return s.getSnapshot(s.store.lastNumber)
}

// getNextCandidate returns a possible candidate from candidates list
func (s *SnapshotValidatorStore) getNextCandidate(
	snap *Snapshot,
	proposer types.Address,
) *store.Candidate {
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
func (s *SnapshotValidatorStore) cleanObsoleteCandidates(set validators.Validators) {
	newCandidates := make([]*store.Candidate, 0, len(s.candidates))

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

// pickOneCandidate returns a proposer candidate from candidates field
// Unsafe against concurrent accesses
func (s *SnapshotValidatorStore) pickOneCandidate(
	snap *Snapshot,
	proposer types.Address,
) *store.Candidate {
	for _, c := range s.candidates {
		addr := c.Validator.Addr()

		count := snap.Count(func(v *store.Vote) bool {
			return v.Candidate.Addr() == addr && v.Validator == proposer
		})

		if count == 0 {
			return c
		}
	}

	return nil
}

// saveSnapshotIfChanged is a helper method to save snapshot updated by the given header
// only if the snapshot is updated from parent snapshot
func (s *SnapshotValidatorStore) saveSnapshotIfChanged(
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
func (s *SnapshotValidatorStore) resetSnapshot(
	parentSnapshot, snapshot *Snapshot,
	header *types.Header,
) {
	snapshot.Votes = nil

	s.saveSnapshotIfChanged(parentSnapshot, snapshot, header)
}

// removeLowerSnapshots is a helper function to removes old snapshots
func (s *SnapshotValidatorStore) removeLowerSnapshots(
	currentHeight uint64,
) {
	currentEpoch := currentHeight / s.epochSize
	if currentEpoch < preservedEpochs {
		return
	}

	// remove in-memory snapshots from two epochs before this one
	lowerEpoch := currentEpoch - preservedEpochs
	purgeBlock := lowerEpoch * s.epochSize
	s.store.deleteLower(purgeBlock)
}

// processVote processes vote in the given header and update snapshot
func processVote(
	snapshot *Snapshot,
	header *types.Header,
	candidateType validators.ValidatorType,
	proposer types.Address,
) error {
	// the nonce selects the action
	authorize, err := isAuthorize(header.Nonce)
	if err != nil {
		return err
	}

	// parse candidate validator set from header.Miner
	candidate, err := minerToValidator(candidateType, header.Miner)
	if err != nil {
		return err
	}

	// if candidate has been processed as expected, just update last block
	if !shouldProcessVote(snapshot.Set, candidate.Addr(), authorize) {
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
