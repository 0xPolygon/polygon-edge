package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

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
	ErrInvalidNonce           = errors.New("invalid nonce specified")
	ErrSnapshotNotFound       = errors.New("not found snapshot")
	ErrUnauthorizedProposer   = errors.New("unauthorized proposer")
	ErrIncorrectNonce         = errors.New("incorrect vote nonce")
	ErrAlreadyCandidate       = errors.New("already a candidate")
	ErrCandidateIsValidator   = errors.New("the candidate is already a validator")
	ErrCandidateNotExistInSet = errors.New("cannot remove a validator if they're not in the snapshot")
	ErrAlreadyVoted           = errors.New("already voted for this address")
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
		// Add genesis
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

		for num := meta.LastBlock + 1; num <= header.Number; num++ {
			if num == 0 {
				continue
			}

			header, ok := s.blockchain.GetHeaderByNumber(num)
			if !ok {
				return fmt.Errorf("header %d not found", num)
			}

			if err := s.ProcessHeader(header); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SnapshotValidatorSet) SourceType() valset.SourceType {
	return valset.Snapshot
}

func (s *SnapshotValidatorSet) GetValidators(height uint64) (validators.Validators, error) {
	var snapshotHeight uint64 = 0
	if int64(height)-1 < 0 {
		snapshotHeight = 0
	} else {
		snapshotHeight = height - 1
	}

	snapshot := s.getSnapshot(snapshotHeight)
	if snapshot == nil {
		return nil, ErrSnapshotNotFound
	}

	return snapshot.Set, nil
}

func (s *SnapshotValidatorSet) GetSnapshotMetadata() *SnapshotMetadata {
	return &SnapshotMetadata{
		LastBlock: s.store.getLastBlock(),
	}
}

func (s *SnapshotValidatorSet) GetSnapshots() []*Snapshot {
	return s.store.list
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
		header.Miner = types.MarshalRLPTo(candidate.Validator.MarshalRLPWith, nil)

		if candidate.Authorize {
			header.Nonce = nonceAuthVote
		} else {
			header.Nonce = nonceDropVote
		}
	}

	return nil
}

func (s *SnapshotValidatorSet) VerifyHeader(header *types.Header) error {
	if header.Nonce != nonceAuthVote && header.Nonce != nonceDropVote {
		return ErrInvalidNonce
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

	// Check if the recovered proposer is part of the validator set
	vals, err := s.GetValidators(header.Number)
	if err != nil {
		return err
	}

	if !vals.Includes(proposer) {
		return ErrUnauthorizedProposer
	}

	parentSnap := s.getSnapshot(header.Number - 1)
	if parentSnap == nil {
		return ErrSnapshotNotFound
	}

	snap := parentSnap.Copy()

	saveSnap := func() {
		if !snap.Equal(parentSnap) {
			snap.Number = header.Number
			snap.Hash = header.Hash.String()

			s.store.add(snap)
		}
	}

	// Reset votes when new epoch
	if header.Number%s.epochSize == 0 {
		snap.Votes = nil

		saveSnap()

		// remove in-memory snapshots from two epochs before this one
		if lowerEpoch := int(header.Number/s.epochSize) - 2; lowerEpoch > 0 {
			purgeBlock := uint64(lowerEpoch) * s.epochSize
			s.store.deleteLower(purgeBlock)
		}

		s.store.updateLastBlock(header.Number)

		return nil
	}

	// Process votes in the middle of epoch
	// if we have a miner address, this might be a vote
	if bytes.Equal(header.Miner, types.ZeroAddress[:]) {
		s.store.updateLastBlock(header.Number)

		return nil
	}

	// the nonce selects the action
	var authorize bool

	switch header.Nonce {
	case nonceAuthVote:
		authorize = true
	case nonceDropVote:
		authorize = false
	default:
		return ErrIncorrectNonce
	}

	validatorType := signer.Type()

	candidate := validators.NewValidatorFromType(validatorType)
	if err := types.UnmarshalRlp(candidate.UnmarshalRLPFrom, header.Miner); err != nil {
		return err
	}

	// validate the vote
	if authorize {
		// we can only authorize if they are not on the validators list
		if snap.Set.Includes(candidate.Addr()) {
			s.store.updateLastBlock(header.Number)

			return nil
		}
	} else {
		// we can only remove if they are part of the validators list
		if !snap.Set.Includes(candidate.Addr()) {
			s.store.updateLastBlock(header.Number)

			return nil
		}
	}

	voteCount := snap.Count(func(v *valset.Vote) bool {
		return v.Validator == proposer && v.Candidate.Equal(candidate)
	})

	if voteCount > 1 {
		// there can only be one vote per validator per address
		return fmt.Errorf("more than one proposal per validator per address found")
	}

	if voteCount == 0 {
		// cast the new vote since there is no one yet
		snap.Votes = append(snap.Votes, &valset.Vote{
			Validator: proposer,
			Candidate: candidate,
			Authorize: authorize,
		})
	}

	// check the tally for the proposed validator
	tally := snap.Count(func(v *valset.Vote) bool {
		return v.Candidate.Equal(candidate)
	})

	// If more than a half of all validators voted
	if tally > snap.Set.Len()/2 {
		if authorize {
			// add the candidate to the validators list
			if err := snap.Set.Add(candidate); err != nil {
				return err
			}
		} else {
			// remove the candidate from the validators list
			if err := snap.Set.Del(candidate); err != nil {
				return err
			}

			// remove any votes casted by the removed validator
			snap.RemoveVotes(func(v *valset.Vote) bool {
				return v.Validator == candidate.Addr()
			})
		}

		// remove all the votes that promoted this validator
		snap.RemoveVotes(func(v *valset.Vote) bool {
			return v.Candidate.Equal(candidate)
		})
	}

	saveSnap()
	s.store.updateLastBlock(header.Number)

	return nil
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
	count := snap.Count(func(v *valset.Vote) bool {
		return v.Candidate.Addr() == candidateAddr && v.Validator == proposer
	})

	if count == 1 {
		return ErrAlreadyVoted
	}

	if auth {
		s.candidates = append(s.candidates, &valset.Candidate{
			Validator: candidate,
			Authorize: auth,
		})
	} else {
		// get candidate validator information from set
		// because don't want user to specify data except for address
		// in case of removal
		validatorIndex := snap.Set.Index(candidate.Addr())
		validatorInSet := snap.Set.At(uint64(validatorIndex))

		s.candidates = append(s.candidates, &valset.Candidate{
			Validator: validatorInSet,
			Authorize: auth,
		})
	}

	return nil
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

// addHeaderSnap creates the initial snapshot, and adds it to the snapshot store
func (s *SnapshotValidatorSet) addHeaderSnap(header *types.Header) error {
	// Genesis header needs to be set by hand, all the other
	// snapshots are set as part of processHeaders
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
	snap := &Snapshot{
		Hash:   header.Hash.String(),
		Number: header.Number,
		Votes:  []*valset.Vote{},
		Set:    extra.Validators,
	}

	s.store.add(snap)

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
	for i := 0; i < len(s.candidates); i++ {
		addr := s.candidates[i].Validator.Addr()

		// Define the delete callback method
		deleteFn := func() {
			s.candidates = append(s.candidates[:i], s.candidates[i+1:]...)
			i--
		}

		// Delete candidate if the candidate has been processed already
		if s.candidates[i].Authorize == snap.Set.Includes(addr) {
			deleteFn()
		}
	}

	var candidate *valset.Candidate

	// now pick the first candidate that has not received a vote yet
	for _, c := range s.candidates {
		addr := c.Validator.Addr()

		count := snap.Count(func(v *valset.Vote) bool {
			return v.Candidate.Addr() == addr && v.Validator == proposer
		})

		if count == 0 {
			// Candidate found
			candidate = c

			break
		}
	}

	return candidate
}
