package snapshot

import (
	"encoding/json"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
)

// snapshotMetadata defines the metadata for the snapshot
type SnapshotMetadata struct {
	// LastBlock represents the latest block in the snapshot
	LastBlock uint64
}

// Snapshot is the current state at a given point in time for validators and votes
type Snapshot struct {
	// block number when the snapshot was created
	Number uint64

	// block hash when the snapshot was created
	Hash string

	// votes casted in chronological order
	Votes []*valset.Vote

	// current set of validators
	Set validators.Validators
}

func (s *Snapshot) MarshalJSON() ([]byte, error) {
	jsonData := struct {
		Number uint64
		Hash   string
		Votes  []*valset.Vote
		Type   validators.ValidatorType
		Set    validators.Validators
	}{
		Number: s.Number,
		Hash:   s.Hash,
		Votes:  s.Votes,
		Type:   s.Set.Type(),
		Set:    s.Set,
	}

	return json.Marshal(jsonData)
}

func (s *Snapshot) UnmarshalJSON(data []byte) error {
	raw := struct {
		Number uint64
		Hash   string
		Type   string
		Votes  []json.RawMessage
		Set    json.RawMessage
	}{}

	var err error

	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	s.Number = raw.Number
	s.Hash = raw.Hash

	valType := validators.ECDSAValidatorType
	if len(raw.Type) > 0 {
		if valType, err = validators.ParseValidatorType(raw.Type); err != nil {
			return err
		}
	}

	s.Set = validators.NewValidatorsFromType(valType)

	// Votes
	votes := make([]*valset.Vote, len(raw.Votes))
	for idx := range votes {
		candidate, err := validators.NewValidatorFromType(valType)
		if err != nil {
			return err
		}

		votes[idx] = &valset.Vote{
			Candidate: candidate,
		}

		if err := json.Unmarshal(raw.Votes[idx], votes[idx]); err != nil {
			return err
		}
	}

	s.Votes = votes

	// Set
	if err := json.Unmarshal(raw.Set, s.Set); err != nil {
		return err
	}

	return nil
}

// Equal checks if two snapshots are equal
func (s *Snapshot) Equal(ss *Snapshot) bool {
	// we only check if Votes and Set are equal since Number and Hash
	// are only meant to be used for indexing
	if len(s.Votes) != len(ss.Votes) {
		return false
	}

	for indx := range s.Votes {
		if !s.Votes[indx].Equal(ss.Votes[indx]) {
			return false
		}
	}

	return s.Set.Equal(ss.Set)
}

// Count returns the vote tally.
// The count increases if the callback function returns true
func (s *Snapshot) Count(h func(v *valset.Vote) bool) (count int) {
	for _, v := range s.Votes {
		if h(v) {
			count++
		}
	}

	return
}

// AddVote adds a vote to snapshot
func (s *Snapshot) AddVote(
	voter types.Address,
	candidate validators.Validator,
	authorize bool,
) {
	s.Votes = append(s.Votes, &valset.Vote{
		Validator: voter,
		Candidate: candidate,
		Authorize: authorize,
	})
}

// Copy makes a copy of the snapshot
func (s *Snapshot) Copy() *Snapshot {
	// Do not need to copy Number and Hash
	ss := &Snapshot{
		Votes: make([]*valset.Vote, len(s.Votes)),
		Set:   s.Set.Copy(),
	}

	for indx, vote := range s.Votes {
		ss.Votes[indx] = vote.Copy()
	}

	return ss
}

// CountByCandidateAndVoter is a helper method to count votes by voter address and candidate
func (s *Snapshot) CountByVoterAndCandidate(
	voter types.Address,
	candidate validators.Validator,
) int {
	return s.Count(func(v *valset.Vote) bool {
		return v.Validator == voter && v.Candidate.Equal(candidate)
	})
}

// CountByCandidateAndVoter is a helper method to count votes by candidate
func (s *Snapshot) CountByCandidate(
	candidate validators.Validator,
) int {
	return s.Count(func(v *valset.Vote) bool {
		return v.Candidate.Equal(candidate)
	})
}

// RemoveVotes removes the Votes that meet condition defined in the given function
func (s *Snapshot) RemoveVotes(shouldRemoveFn func(v *valset.Vote) bool) {
	newVotes := make([]*valset.Vote, 0, len(s.Votes))

	for _, vote := range s.Votes {
		if shouldRemoveFn(vote) {
			continue
		}

		newVotes = append(newVotes, vote)
	}

	// match capacity with size in order to shrink array
	s.Votes = newVotes[:len(newVotes):len(newVotes)]
}

// RemoveVotesByVoter is a helper method to remove all votes created by specified address
func (s *Snapshot) RemoveVotesByVoter(
	address types.Address,
) {
	s.RemoveVotes(func(v *valset.Vote) bool {
		return v.Validator == address
	})
}

// RemoveVotesByCandidate is a helper method to remove all votes to specified candidate
func (s *Snapshot) RemoveVotesByCandidate(
	candidate validators.Validator,
) {
	s.RemoveVotes(func(v *valset.Vote) bool {
		return v.Candidate.Equal(candidate)
	})
}

// snapshotStore defines the structure of the stored snapshots
type snapshotStore struct {
	sync.RWMutex

	// lastNumber is the latest block number stored
	lastNumber uint64

	// list represents the actual snapshot sorted list
	list snapshotSortedList
}

// newSnapshotStore returns a new snapshot store
func newSnapshotStore(
	metadata *SnapshotMetadata,
	snapshots []*Snapshot,
) *snapshotStore {
	store := &snapshotStore{
		list: snapshotSortedList{},
	}

	store.loadData(metadata, snapshots)

	return store
}

func (s *snapshotStore) loadData(
	metadata *SnapshotMetadata,
	snapshots []*Snapshot,
) {
	if metadata != nil {
		s.lastNumber = metadata.LastBlock
	}

	for _, snap := range snapshots {
		s.add(snap)
	}
}

// getLastBlock returns the latest block number from the snapshot store. [Thread safe]
func (s *snapshotStore) getLastBlock() uint64 {
	return atomic.LoadUint64(&s.lastNumber)
}

// updateLastBlock sets the latest block number in the snapshot store. [Thread safe]
func (s *snapshotStore) updateLastBlock(num uint64) {
	atomic.StoreUint64(&s.lastNumber, num)
}

// deleteLower deletes snapshots that have a block number lower than the passed in parameter
func (s *snapshotStore) deleteLower(num uint64) {
	s.Lock()
	defer s.Unlock()

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].Number >= num
	})

	s.list = s.list[i:]
}

// find returns the index of the first closest snapshot to the number specified
func (s *snapshotStore) find(num uint64) *Snapshot {
	s.RLock()
	defer s.RUnlock()

	if len(s.list) == 0 {
		return nil
	}

	// fast track, check the last item
	if last := s.list[len(s.list)-1]; last.Number < num {
		return last
	}

	i := sort.Search(len(s.list), func(i int) bool {
		return s.list[i].Number >= num
	})

	if i < len(s.list) {
		if i == 0 {
			return s.list[0]
		}

		if s.list[i].Number == num {
			return s.list[i]
		}

		return s.list[i-1]
	}

	return nil
}

// add adds a new snapshot to the snapshot store
func (s *snapshotStore) add(snap *Snapshot) {
	s.Lock()
	defer s.Unlock()

	// append and sort the list
	s.list = append(s.list, snap)
	sort.Sort(&s.list)
}

// snapshotSortedList defines the sorted snapshot list
type snapshotSortedList []*Snapshot

// Len returns the size of the sorted snapshot list
func (s snapshotSortedList) Len() int {
	return len(s)
}

// Swap swaps two values in the sorted snapshot list
func (s snapshotSortedList) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less checks if the element at index I has a lower number than the element at index J
func (s snapshotSortedList) Less(i, j int) bool {
	return s[i].Number < s[j].Number
}
